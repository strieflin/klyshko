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
                "mp-spdz",
                "operator",
                "operator-helm-chart",
                "provisioner"
            ]
        ]
    }
}
