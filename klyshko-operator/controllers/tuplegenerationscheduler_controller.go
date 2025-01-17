/*
Copyright (c) 2022 - for information on the respective copyright owner
see the NOTICE file and/or the repository https://github.com/carbynestack/klyshko.

SPDX-License-Identifier: Apache-2.0
*/

package controllers

import (
	"context"
	"fmt"
	"github.com/carbynestack/klyshko/castor"
	"github.com/carbynestack/klyshko/logging"
	"github.com/google/uuid"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"math/rand"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	klyshkov1alpha1 "github.com/carbynestack/klyshko/api/v1alpha1"
)

// MinimumTuplesPerJob is the minimum number of tuples generated by a TupleGenerationJob.
const MinimumTuplesPerJob = 10000

// PeriodicReconciliationDuration is the maximum time between two successive reconciliations
const PeriodicReconciliationDuration = 10 * time.Second

// TupleGenerationSchedulerReconciler reconciles a TupleGenerationScheduler object.
type TupleGenerationSchedulerReconciler struct {
	client.Client
	Scheme       *runtime.Scheme
	CastorClient *castor.Client
}

//+kubebuilder:rbac:groups=klyshko.carbnyestack.io,resources=tuplegenerationschedulers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=klyshko.carbnyestack.io,resources=tuplegenerationschedulers/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=klyshko.carbnyestack.io,resources=tuplegenerationschedulers/finalizers,verbs=update

// Reconcile compares the actual state of TupleGenerationScheduler resources to their desired state and performs actions
// to bring the actual state closer to the desired one.
func (r *TupleGenerationSchedulerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.V(logging.DEBUG).Info("Reconciling tuple generation schedulers")

	// Fetch scheduler resource
	scheduler := &klyshkov1alpha1.TupleGenerationScheduler{}
	err := r.Get(ctx, req.NamespacedName, scheduler)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Scheduler resource not available -> has been deleted
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return ctrl.Result{}, fmt.Errorf("failed to read scheduler resource: %w", err)
	}

	// Remove all finished jobs
	if r.cleanupFinishedJobs(ctx, scheduler) != nil {
		return ctrl.Result{}, fmt.Errorf("failed to delete finished jobs: %w", err)
	}

	// Fetch active jobs
	activeJobs, err := r.getMatchingJobs(ctx, func(job klyshkov1alpha1.TupleGenerationJob) bool {
		return !job.Status.State.IsDone()
	})
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to fetch active jobs: %w", err)
	}

	// Stop if already at maximum concurrency level
	activeJobCount := len(activeJobs)
	if scheduler.Spec.Concurrency <= activeJobCount {
		logger.V(logging.DEBUG).Info("At maximum concurrency level - do nothing", "Jobs.Active", activeJobCount, "Scheduler.Concurrency", scheduler.Spec.Concurrency)
		return ctrl.Result{RequeueAfter: PeriodicReconciliationDuration}, nil
	}

	// Fetch telemetry data from Castor
	telemetry, err := r.CastorClient.GetTelemetry(ctx)
	if err != nil {
		logger.Error(err, "Fetching telemetry data from Castor failed", "Castor.URL", r.CastorClient.URL)
		return ctrl.Result{RequeueAfter: PeriodicReconciliationDuration}, err
	}

	// Decide which tuple type to generate next based on (for now fixed) strategy
	var strategy SchedulingStrategy = &LeastAvailableFirstStrategy{Threshold: scheduler.Spec.Threshold}
	tupleType := strategy.Schedule(ctx, telemetry, activeJobs)
	if tupleType == nil {
		return ctrl.Result{RequeueAfter: PeriodicReconciliationDuration}, nil
	}

	// Create job for selected tuple type
	err = r.createJob(ctx, scheduler, *tupleType)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to create tuple generation job: %w", err)
	}

	return ctrl.Result{
		RequeueAfter: PeriodicReconciliationDuration,
	}, nil
}

// Creates a tuple generation job for the given tuple type in the namespace where the scheduler lives in.
func (r *TupleGenerationSchedulerReconciler) createJob(ctx context.Context, scheduler *klyshkov1alpha1.TupleGenerationScheduler, tupleType string) error {
	logger := log.FromContext(ctx)
	jobID := uuid.New().String()
	job := &klyshkov1alpha1.TupleGenerationJob{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "klyshko.carbnyestack.io/v1alpha1",
			Kind:       "TupleGenerationJob",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      scheduler.Name + "-" + jobID,
			Namespace: scheduler.Namespace,
		},
		Spec: klyshkov1alpha1.TupleGenerationJobSpec{
			ID:        jobID,
			Type:      tupleType,
			Count:     MinimumTuplesPerJob, // TODO Make this configurable
			Generator: scheduler.Spec.Generator,
		},
		Status: klyshkov1alpha1.TupleGenerationJobStatus{
			State:                   klyshkov1alpha1.JobPending,
			LastStateTransitionTime: metav1.Now(),
		},
	}
	err := ctrl.SetControllerReference(scheduler, job, r.Scheme)
	if err != nil {
		logger.Error(err, "could not set owner reference on job", "Job", job)
		return err
	}
	err = r.Create(ctx, job)
	if err != nil {
		logger.Error(err, "job creation failed", "Job", job)
		return err
	}
	logger.Info("Job created", "Job", job)
	return nil
}

// Deletes all jobs that are done, i.e., either complete or failed, and beyond the TTL
func (r *TupleGenerationSchedulerReconciler) cleanupFinishedJobs(ctx context.Context, scheduler *klyshkov1alpha1.TupleGenerationScheduler) error {
	logger := log.FromContext(ctx)
	finishedJobs, err := r.getMatchingJobs(ctx, func(job klyshkov1alpha1.TupleGenerationJob) bool {
		isBeyondTTL := func() bool {
			return time.Now().After(job.Status.LastStateTransitionTime.Add(time.Duration(scheduler.Spec.TTLSecondsAfterFinished) * time.Second))
		}
		return job.Status.State.IsDone() && isBeyondTTL()
	})
	if err != nil {
		logger.Error(err, "failed to fetch finished jobs")
		return err
	}
	logger.V(logging.DEBUG).Info("Deleting finished jobs", "jobs", finishedJobs)
	// Shuffling jobs to ensure that finished jobs do not accumulate while we try to delete
	// the same finished job over and over again
	rand.Shuffle(len(finishedJobs), func(i, j int) {
		finishedJobs[i], finishedJobs[j] = finishedJobs[j], finishedJobs[i]
	})
	for _, j := range finishedJobs {
		err := r.Delete(ctx, &j)
		if err != nil {
			logger.Error(err, "failed to delete finished job", "Job", j)
			return err
		}
	}
	return nil
}

// Returns all jobs that match the given predicate.
func (r *TupleGenerationSchedulerReconciler) getMatchingJobs(ctx context.Context, pred func(klyshkov1alpha1.TupleGenerationJob) bool) ([]klyshkov1alpha1.TupleGenerationJob, error) {
	logger := log.FromContext(ctx)
	allJobs := &klyshkov1alpha1.TupleGenerationJobList{}
	err := r.List(ctx, allJobs)
	if err != nil {
		return nil, err
	}
	logger.V(logging.TRACE).Info("Considering potentially matching jobs", "jobs", allJobs)
	var matchingJobs []klyshkov1alpha1.TupleGenerationJob
	for _, j := range allJobs.Items {
		if pred(j) {
			matchingJobs = append(matchingJobs, j)
		}
	}
	return matchingJobs, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *TupleGenerationSchedulerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&klyshkov1alpha1.TupleGenerationScheduler{}).
		Owns(&klyshkov1alpha1.TupleGenerationJob{}).
		Complete(r)
}
