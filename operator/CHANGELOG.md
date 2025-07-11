# Changelog
All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/)
and is generated by [Changie](https://github.com/miniscruff/changie).


## [v25.1.1-beta3](https://github.com/redpanda-data/redpanda-operator/releases/tag/operator%2Fv25.1.1-beta3) - 2025-05-07
### Added
* Added scheduled sync of ghost broker decommissioner to ensure it's running, even if no watches trigger the reconciler.
* v1 operator: ExternalSecretRefSelector is now provided for referring to external secrets in `clusterConfiguration`. This has an `optional` flag which is honoured if present - it turns errors into warnings if the secret can't be looked up.
### Changed
* [Chart] Moved all template rendering into `entry-point.yaml` to match the redpanda and console charts.
* `values.schema.json` is now "closed" (`additionalProperties: false`)

  Any unexpected values will result in a validation error,previously they would
  have been ignored.
* The redpanda operator's helm chart has been merged into the operator itself.

  Going forward the chart's `version` and `appVersion` will always be equal.
* `rbac.createRPKBundleCRs` now defaults to `true`.
* The operator will now populate `.Statefulset.SideCars.Image`, if unspecified, with it's own image.

  The image and tag may be controlled with pre-existing
  `--configurator-base-image` and `--configurator-tag` flags, respectively.

  The previous behavior was to defer to the default of the redpanda chart which
  could result in out of sync RBAC requirements or regressions of
  sidecar/initcontainer behavior, if using an older redpanda chart.
### Deprecated
* v1 operator: the `clusterConfiguration` field `ExternalSecretRef` is deprecated in favour of `ExternalSecretRefSelector`. Since this field was extremely new, it will be removed in the very near future.
### Removed
* Removed bundled FluxCD controllers, bundled FluxCD CRDs, and support for delegating control to FluxCD.

  Previously reconciled FluxCD resources (`HelmRepository`, `HelmRelease`)
  will **NOT** be garbage collected upon upgrading. If the operator is
  coexisting with a FluxCD installation, please take care to manually remove
  the left over resources.

  `chartRef.useFlux: true` and `chartRef.chartVersion` are no longer
  supported. The controller will log errors and abort reconcilation until the
  fields are unset. Ensure that both have been removed from all `Redpanda`
  resources before upgrading.

  All other `chartRef` fields are deprecated and are no longer referenced.

  `helmRelease`, `helmReleaseReady`, `helmRepository`, `helmRepositoryReady`,
  and `upgradeFailures` are no longer set on `RedpandaStatus`, similar to their
  behavior when `useFlux: false` was set.
* `gcr.io/kubebuilder/kube-rbac-proxy` container is deprecated and has been removed from the Redpanda
operator helm chart. The same ports will continue to serve metrics using kubebuilder's built in RBAC.

  Any existing prometheus rules don't need to be adjusted.

  For more details see: https://github.com/kubernetes-sigs/kubebuilder/discussions/3907

* The V1 operator now requires a minimum Redpanda version of 23.2; all feature-gated behaviour that supported older versions is now enabled unconditionally.
* The [`kube-prometheus-stack`](https://prometheus-community.github.io/helm-charts) subchart has been removed.

  This integration was not being up kept and most use cases will be better served by deploying this chart themselves.
### Fixed
* Certificate reloading for webhook and metrics endpoints should now behave correctly.
* The operator will restart the redpanda cluster on any change to the cluster configuration
* Expanded the set of rules in both Roles and ClusterRoles to be appropriately in sync with the redpanda helm chart.
* DeprecatedFullNameOverride was interpreted differently between rendering resources and creating 
  kafka, admin and schema registry client. Now deprecated fullNameOverride will be used only
  if correct FullNameOverride is not provided and handled the same way for both
  client creation and render function.
* The Redpanda license was not set by operator. Now it will be set in the first reconciliation. After initial setup the consequent license re-set will be reconciled after client-go cache resync timeout (default 10h).
* The operator now unconditionally produces statefulsets that have environment variables available to the initContainer that are used for CEL-based config patching.

Previously it attempted to leave existing sts resources unpatched if it seemed like they had already been bootstrapped. With the adoption of CEL patching for node configuration, that left sts pods unable to restart.
* The operator now unconditionally produces an environment for the initContainer that supports CEL-based patching.

This is required to ensure that a pre-existing sts can roll over to new configuration correctly.

## [v25.1.1-beta2](https://github.com/redpanda-data/redpanda-operator/releases/tag/operator%2Fv25.1.1-beta2) - 2025-04-24
### Added
* Added scheduled sync of ghost broker decommissioner to ensure it's running, even if no watches trigger the reconciler.
### Changed
* [Chart] Moved all template rendering into `entry-point.yaml` to match the redpanda and console charts.
* `values.schema.json` is now "closed" (`additionalProperties: false`)

  Any unexpected values will result in a validation error,previously they would
  have been ignored.
* The redpanda operator's helm chart has been merged into the operator itself.

  Going forward the chart's `version` and `appVersion` will always be equal.
* `rbac.createRPKBundleCRs` now defaults to `true`.
### Removed
* Removed bundled FluxCD controllers, bundled FluxCD CRDs, and support for delegating control to FluxCD.

  Previously reconciled FluxCD resources (`HelmRepository`, `HelmRelease`)
  will **NOT** be garbage collected upon upgrading. If the operator is
  coexisting with a FluxCD installation, please take care to manually remove
  the left over resources.

  `chartRef.useFlux: true` and `chartRef.chartVersion` are no longer
  supported. The controller will log errors and abort reconcilation until the
  fields are unset. Ensure that both have been removed from all `Redpanda`
  resources before upgrading.

  All other `chartRef` fields are deprecated and are no longer referenced.

  `helmRelease`, `helmReleaseReady`, `helmRepository`, `helmRepositoryReady`,
  and `upgradeFailures` are no longer set on `RedpandaStatus`, similar to their
  behavior when `useFlux: false` was set.
* `gcr.io/kubebuilder/kube-rbac-proxy` container is deprecated and has been removed from the Redpanda
operator helm chart. The same ports will continue to serve metrics using kubebuilder's built in RBAC.

  Any existing prometheus rules don't need to be adjusted.

  For more details see: https://github.com/kubernetes-sigs/kubebuilder/discussions/3907

* The V1 operator now requires a minimum Redpanda version of 23.2; all feature-gated behaviour that supported older versions is now enabled unconditionally.
* The [`kube-prometheus-stack`](https://prometheus-community.github.io/helm-charts) subchart has been removed.

  This integration was not being up kept and most use cases will be better served by deploying this chart themselves.
### Fixed
* Certificate reloading for webhook and metrics endpoints should now behave correctly.
* The operator will restart the redpanda cluster on any change to the cluster configuration
* Expanded the set of rules in both Roles and ClusterRoles to be appropriately in sync with the redpanda helm chart.
* DeprecatedFullNameOverride was interpreted differently between rendering resources and creating 
  kafka, admin and schema registry client. Now deprecated fullNameOverride will be used only
  if correct FullNameOverride is not provided and handled the same way for both
  client creation and render function.

## v25.1.1-beta1 - 2025-04-10
### Added
* Added scheduled sync of ghost broker decommissioner to ensure it's running, even if no watches trigger the reconciler.
### Changed
* Bumped internal redpanda chart to  v5.9.19.
  `chartRef` now defaults to v5.9.19.
  When `useFlux` is `false`, the equivalent of chart v5.9.19 will be deployed.

* Bumped the internal chart version to v5.9.20.
* [Chart] Moved all template rendering into `entry-point.yaml` to match the redpanda and console charts.
* The redpanda operator's helm chart has been merged into the operator itself.

  Going forward the chart's `version` and `appVersion` will always be equal.
### Removed
* Removed bundled FluxCD controllers, bundled FluxCD CRDs, and support for delegating control to FluxCD.

  Previously reconciled FluxCD resources (`HelmRepository`, `HelmRelease`)
  will **NOT** be garbage collected upon upgrading. If the operator is
  coexisting with a FluxCD installation, please take care to manually remove
  the left over resources.

  `chartRef.useFlux: true` and `chartRef.chartVersion` are no longer
  supported. The controller will log errors and abort reconcilation until the
  fields are unset. Ensure that both have been removed from all `Redpanda`
  resources before upgrading.

  All other `chartRef` fields are deprecated and are no longer referenced.

  `helmRelease`, `helmReleaseReady`, `helmRepository`, `helmRepositoryReady`,
  and `upgradeFailures` are no longer set on `RedpandaStatus`, similar to their
  behavior when `useFlux: false` was set.
* `gcr.io/kubebuilder/kube-rbac-proxy` container is deprecated and has been removed from the Redpanda
operator helm chart. The same ports will continue to serve metrics using kubebuilder's built in RBAC.

Any existing prometheus rules don't need to be adjusted.

For more details see: https://github.com/kubernetes-sigs/kubebuilder/discussions/3907

* The V1 operator now requires a minimum Redpanda version of 23.2; all feature-gated behaviour that supported older versions is now enabled unconditionally.
### Fixed
* Usage of `tpl` and `include` now function as expected when `useFlux: false` is set.

  `{{ (get (fromJson (include "redpanda.Fullname" (dict "a" (list .)))) "r") }}` would previously failure with fairly arcane errors.

  Now, the above example will correctly render to a string value. However,
  syntax errors and the like are still reported in an arcane fashion.

* Toggling `useFlux`, in either direction, no longer causes the bootstrap user's password to be regenerated.

  Manual mitigation steps are available [here](https://github.com/redpanda-data/helm-charts/issues/1596#issuecomment-2628356953).
* Certificate reloading for webhook and metrics endpoints should now behave correctly.
* Expanded the set of rules in both Roles and ClusterRoles to be appropriately in sync with the redpanda helm chart.

## v2.3.8-24.3.6 - 2025-03-05
### Fixed
* Fixed the way that paths are handled for the config watcher routine in the sidecar process.

## v2.3.6-24.3.3 - 2025-01-17
### Added
* Users in air-gapped environments that cannot access the official Redpanda Helm Chart repository (`https://charts.redpanda.com/`)
  can now specify an alternative Helm chart repository using the `helm-repository-url` flag. In the Redpanda Operator Helm chart,
  this flag is not exposed as an option in the Helm values. Instead, it must be set as an input in the `additionalCmdFlags` array.
  
  The given repository must include the following charts:
  * Redpanda
  * Console
  * Connectors

* Added `resources.limits` and `resources.requests` as an alternative method of managing the redpanda container's resources.

  When both `resources.limits` and `resources.requests` are specified, the
  redpanda container's `resources` will be set to the provided values and all
  other keys of `resources` will be ignored. Instead, all other values will be
  inferred from the limits and requests.

  This allows fine grain control of resources. i.e. It is now possible to set
  CPU requests without setting limits:

  ```yaml
  resources:
    limits: {} # Specified but no cpu or memory values provided
    requests:
      cpu: 5 # Only CPU requests
  ```

### Changed
* For any user that is mirroring configurator image (air-gapped environment) and changes entrypoint
  or wraps configurator with additional script the following constraint need to be meet:
  * set the following flags
    * to change the container repository set `--configurator-base-image=my.repo.com/configurator` flag
    * to change the container tag set `--configurator-tag=XYZ` flag
  * image needs to supports the entrypoint `redpanda-operator configure` as it is the default one

### Fixed
* Value's merging no longer writes files to disk which prevents the operator from eating disk space when the reconciliation loop is run in rapid succession
* Fixed slice out of bounds panics when using the fs-validator and `useFlux: false`

