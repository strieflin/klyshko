module.exports = {
    extends: [
        "@commitlint/config-conventional"
    ],
    rules: {
        'scope-empty': [0, 'never'],
        "scope-enum": [
            2,
            "always",
            [
                "klyshko-mp-spdz",
                "klyshko-operator",
                "klyshko-operator-helm-chart",
                "klyshko-provisioner"
            ]
        ]
    }
}
