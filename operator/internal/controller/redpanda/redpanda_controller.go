// Copyright 2024 Redpanda Data, Inc.
//
// Use of this software is governed by the Business Source License
// included in the file licenses/BSL.md
//
// As of the Change Date specified in that file, in accordance with
// the Business Source License, use of this software will be governed
// by the Apache License, Version 2.0

// Package redpanda contains reconciliation logic for cluster.redpanda.com CRDs
package redpanda

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	helmv2beta1 "github.com/fluxcd/helm-controller/api/v2beta1"
	helmv2beta2 "github.com/fluxcd/helm-controller/api/v2beta2"
	"github.com/fluxcd/pkg/apis/meta"
	"github.com/fluxcd/pkg/runtime/logger"
	sourcev1 "github.com/fluxcd/source-controller/api/v1beta2"
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	kuberecorder "k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/redpanda-data/helm-charts/charts/redpanda"
	"github.com/redpanda-data/helm-charts/pkg/gotohelm/helmette"
	"github.com/redpanda-data/helm-charts/pkg/kube"
	"github.com/redpanda-data/redpanda-operator/operator/api/redpanda/v1alpha2"
	internalclient "github.com/redpanda-data/redpanda-operator/operator/pkg/client"
	"github.com/redpanda-data/redpanda-operator/operator/pkg/resources"
)

const (
	FinalizerKey = "operator.redpanda.com/finalizer"

	NotManaged = "false"

	resourceReadyStrFmt    = "%s '%s/%s' is ready"
	resourceNotReadyStrFmt = "%s '%s/%s' is not ready"

	resourceTypeHelmRepository = "HelmRepository"
	resourceTypeHelmRelease    = "HelmRelease"

	managedPath = "/managed"

	revisionPath        = "/revision"
	componentLabelValue = "redpanda-statefulset"
)

var errWaitForReleaseDeletion = errors.New("wait for helm release deletion")

type gvkKey struct {
	GVK schema.GroupVersionKind
	Key client.ObjectKey
}

// RedpandaReconciler reconciles a Redpanda object
type RedpandaReconciler struct {
	Client        client.Client
	Scheme        *runtime.Scheme
	EventRecorder kuberecorder.EventRecorder
	ClientFactory internalclient.ClientFactory
}

// flux resources main resources
// +kubebuilder:rbac:groups=helm.toolkit.fluxcd.io,namespace=default,resources=helmreleases,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=helm.toolkit.fluxcd.io,namespace=default,resources=helmreleases/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=helm.toolkit.fluxcd.io,namespace=default,resources=helmreleases/finalizers,verbs=update
// +kubebuilder:rbac:groups=source.toolkit.fluxcd.io,namespace=default,resources=helmcharts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=source.toolkit.fluxcd.io,namespace=default,resources=helmcharts/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=source.toolkit.fluxcd.io,namespace=default,resources=helmcharts/finalizers,verbs=get;create;update;patch;delete
// +kubebuilder:rbac:groups=source.toolkit.fluxcd.io,namespace=default,resources=helmrepositories,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=source.toolkit.fluxcd.io,namespace=default,resources=helmrepositories/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=source.toolkit.fluxcd.io,namespace=default,resources=helmrepositories/finalizers,verbs=get;create;update;patch;delete
// +kubebuilder:rbac:groups=source.toolkit.fluxcd.io,namespace=default,resources=gitrepository,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=source.toolkit.fluxcd.io,namespace=default,resources=gitrepository/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=source.toolkit.fluxcd.io,namespace=default,resources=gitrepository/finalizers,verbs=get;create;update;patch;delete

// flux additional resources
// +kubebuilder:rbac:groups=source.toolkit.fluxcd.io,namespace=default,resources=buckets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=source.toolkit.fluxcd.io,namespace=default,resources=gitrepositories,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,namespace=default,resources=replicasets,verbs=get;list;watch;create;update;patch;delete

// any resource that Redpanda helm creates and flux controller needs to reconcile them
// +kubebuilder:rbac:groups="",namespace=default,resources=pods,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,namespace=default,resources=rolebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,namespace=default,resources=roles,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=batch,namespace=default,resources=jobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,namespace=default,resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,namespace=default,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,namespace=default,resources=serviceaccounts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,namespace=default,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=policy,namespace=default,resources=poddisruptionbudgets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,namespace=default,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cert-manager.io,namespace=default,resources=certificates,verbs=get;create;update;patch;delete;list;watch
// +kubebuilder:rbac:groups=cert-manager.io,namespace=default,resources=issuers,verbs=get;create;update;patch;delete;list;watch
// +kubebuilder:rbac:groups="monitoring.coreos.com",namespace=default,resources=servicemonitors,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.k8s.io,namespace=default,resources=ingresses,verbs=get;list;watch;create;update;patch;delete

// redpanda resources
// +kubebuilder:rbac:groups=cluster.redpanda.com,namespace=default,resources=redpandas,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cluster.redpanda.com,namespace=default,resources=redpandas/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=cluster.redpanda.com,namespace=default,resources=redpandas/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,namespace=default,resources=events,verbs=create;patch

// SetupWithManager sets up the controller with the Manager.
func (r *RedpandaReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager) error {
	if err := registerHelmReferencedIndex(ctx, mgr, "statefulset", &appsv1.StatefulSet{}); err != nil {
		return err
	}
	if err := registerHelmReferencedIndex(ctx, mgr, "deployment", &appsv1.Deployment{}); err != nil {
		return err
	}

	helmManagedComponentPredicate, err := predicate.LabelSelectorPredicate(
		metav1.LabelSelector{
			MatchExpressions: []metav1.LabelSelectorRequirement{{
				Key:      "app.kubernetes.io/name", // look for only redpanda or console pods
				Operator: metav1.LabelSelectorOpIn,
				Values:   []string{"redpanda", "console"},
			}, {
				Key:      "app.kubernetes.io/instance", // make sure we have a cluster name
				Operator: metav1.LabelSelectorOpExists,
			}, {
				Key:      "batch.kubernetes.io/job-name", // filter out the job pods since they also have name=redpanda
				Operator: metav1.LabelSelectorOpDoesNotExist,
			}},
		},
	)
	if err != nil {
		return err
	}

	managedWatchOption := builder.WithPredicates(helmManagedComponentPredicate)

	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha2.Redpanda{}).
		Owns(&sourcev1.HelmRepository{}).
		Owns(&helmv2beta1.HelmRelease{}).
		Owns(&helmv2beta2.HelmRelease{}).
		Watches(&appsv1.StatefulSet{}, enqueueClusterFromHelmManagedObject(), managedWatchOption).
		Watches(&appsv1.Deployment{}, enqueueClusterFromHelmManagedObject(), managedWatchOption).
		Complete(r)
}

func (r *RedpandaReconciler) Reconcile(c context.Context, req ctrl.Request) (ctrl.Result, error) {
	ctx, done := context.WithCancel(c)
	defer done()

	start := time.Now()
	log := ctrl.LoggerFrom(ctx).WithName("RedpandaReconciler.Reconcile")

	defer func() {
		durationMsg := fmt.Sprintf("reconciliation finished in %s", time.Since(start).String())
		log.Info(durationMsg)
	}()

	log.Info("Starting reconcile loop")

	rp := &v1alpha2.Redpanda{}
	if err := r.Client.Get(ctx, req.NamespacedName, rp); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Examine if the object is under deletion
	if !rp.ObjectMeta.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, rp)
	}

	if !isRedpandaManaged(ctx, rp) {
		if controllerutil.ContainsFinalizer(rp, FinalizerKey) {
			// if no longer managed by us, attempt to remove the finalizer
			controllerutil.RemoveFinalizer(rp, FinalizerKey)
			if err := r.Client.Update(ctx, rp); err != nil {
				return ctrl.Result{}, err
			}
		}

		return ctrl.Result{}, nil
	}

	_, ok := rp.GetAnnotations()[resources.ManagedDecommissionAnnotation]
	if ok {
		log.Info("Managed decommission")
		return ctrl.Result{}, nil
	}

	// add finalizer if not exist
	if !controllerutil.ContainsFinalizer(rp, FinalizerKey) {
		patch := client.MergeFrom(rp.DeepCopy())
		controllerutil.AddFinalizer(rp, FinalizerKey)
		if err := r.Client.Patch(ctx, rp, patch); err != nil {
			log.Error(err, "unable to register finalizer")
			return ctrl.Result{}, err
		}
	}

	rp, err := r.reconcile(ctx, rp)
	if err != nil {
		return ctrl.Result{}, err
	}

	err = r.reconcileDefluxed(ctx, rp)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Reconciliation has completed without any errors, therefore we observe our
	// generation and persist any status changes.
	rp.Status.ObservedGeneration = rp.Generation

	// Update status after reconciliation.
	if updateStatusErr := r.patchRedpandaStatus(ctx, rp); updateStatusErr != nil {
		log.Error(updateStatusErr, "unable to update status after reconciliation")
		return ctrl.Result{}, updateStatusErr
	}

	return ctrl.Result{}, nil
}

func (r *RedpandaReconciler) reconcileDefluxed(ctx context.Context, rp *v1alpha2.Redpanda) error {
	log := ctrl.LoggerFrom(ctx)
	log.WithName("RedpandaReconciler.reconcileDefluxed")

	if ptr.Deref(rp.Spec.ChartRef.UseFlux, true) {
		log.Info("useFlux is true; skipping non-flux reconciliation...")
		return nil
	}

	chartVersion := rp.Spec.ChartRef.ChartVersion
	desiredChartVersion := redpanda.Chart.Metadata().Version

	if !(chartVersion == "" || chartVersion == desiredChartVersion) {
		msg := fmt.Sprintf(".spec.chartRef.chartVersion needs to be %q or %q. got %q", desiredChartVersion, "", chartVersion)

		// NB: passing `nil` as err is acceptable for log.Error.
		log.Error(nil, msg, "chart version", rp.Spec.ChartRef.ChartVersion)
		r.EventRecorder.Eventf(rp, "Warning", v1alpha2.EventSeverityError, msg)

		v1alpha2.RedpandaNotReady(rp, "ChartRefUnsupported", msg)

		// Do not error out to not requeue. User needs to first migrate helm release to either "" or the pinned chart's version.
		return nil
	}

	// DeepCopy values to prevent any accidental mutations that may occur
	// within the chart itself.
	values := rp.Spec.ClusterSpec.DeepCopy()

	objs, err := redpanda.Chart.Render(kube.Config{}, helmette.Release{
		Namespace: rp.Namespace,
		Name:      rp.GetHelmReleaseName(),
		Service:   "Helm",
	}, values)
	if err != nil {
		return err
	}

	// set for tracking which objects are expected to exist in this reconciliation run.
	created := make(map[gvkKey]struct{}, len(objs))
	for _, obj := range objs {
		// Namespace is inconsistently set across all our charts. Set it
		// explicitly here to be safe.
		obj.SetNamespace(rp.Namespace)
		obj.SetOwnerReferences([]metav1.OwnerReference{rp.OwnerShipRefObj()})

		labels := obj.GetLabels()
		if labels == nil {
			labels = map[string]string{}
		}

		annos := obj.GetAnnotations()
		if annos == nil {
			annos = map[string]string{}
		}

		// Needed for interop with flux.
		// Without these the flux controller will refuse to take ownership.
		annos["meta.helm.sh/release-name"] = rp.GetHelmReleaseName()
		annos["meta.helm.sh/release-namespace"] = rp.Namespace

		labels["helm.toolkit.fluxcd.io/name"] = rp.GetHelmReleaseName()
		labels["helm.toolkit.fluxcd.io/namespace"] = rp.Namespace

		obj.SetLabels(labels)
		obj.SetAnnotations(annos)

		if _, ok := annos["helm.sh/hook"]; ok {
			log.Info(fmt.Sprintf("skipping helm hook %T: %q", obj, obj.GetName()))
			continue
		}

		// TODO: how to handle immutable issues?
		if err := r.apply(ctx, obj); err != nil {
			return errors.Wrapf(err, "deploying %T: %q", obj, obj.GetName())
		}

		log.Info(fmt.Sprintf("deployed %T: %q", obj, obj.GetName()))

		// Record creation
		created[gvkKey{
			Key: client.ObjectKeyFromObject(obj),
			GVK: obj.GetObjectKind().GroupVersionKind(),
		}] = struct{}{}
	}

	// If our ObservedGeneration is up to date, .Spec hasn't changed since the
	// last successful reconciliation so everything that we'd do here is likely
	// to be a no-op.
	// This check could likely be hoisted above the deployment loop as well.
	if rp.Generation == rp.Status.ObservedGeneration && rp.Generation != 0 {
		log.Info("observed generation is up to date. skipping garbage collection", "generation", rp.Generation, "observedGeneration", rp.Status.ObservedGeneration)
		return nil
	}

	// Garbage collect any objects that are no longer needed.
	if err := r.reconcileDefluxGC(ctx, rp, created); err != nil {
		return err
	}

	return nil
}

func (r *RedpandaReconciler) reconcileDefluxGC(ctx context.Context, rp *v1alpha2.Redpanda, created map[gvkKey]struct{}) error {
	log := ctrl.LoggerFrom(ctx)

	types, err := allListTypes(r.Client)
	if err != nil {
		return err
	}

	// For all types in the redpanda helm chart,
	var toDelete []kube.Object
	for _, typ := range types {
		// Find all objects that have flux's internal label selector.
		if err := r.Client.List(ctx, typ, client.InNamespace(rp.Namespace), client.MatchingLabels{
			"helm.toolkit.fluxcd.io/name":      rp.GetHelmReleaseName(),
			"helm.toolkit.fluxcd.io/namespace": rp.Namespace,
		}); err != nil {
			// Some types from 3rd parties (monitoring, cert-manager) may not
			// exists. If they don't skip over them without erroring out.
			if apimeta.IsNoMatchError(err) {
				log.Info("Skipping unknown GVK", "gvk", typ)
				continue
			}
			return err
		}

		if err := apimeta.EachListItem(typ, func(o runtime.Object) error {
			obj := o.(client.Object)

			gvk, err := r.Client.GroupVersionKindFor(obj)
			if err != nil {
				return errors.WithStack(err)
			}

			key := gvkKey{Key: client.ObjectKeyFromObject(obj), GVK: gvk}

			isOwned := -1 != slices.IndexFunc(obj.GetOwnerReferences(), func(owner metav1.OwnerReference) bool {
				return owner.UID == rp.UID
			})

			// If we've just created this object, don't consider it for
			// deletion.
			if _, ok := created[key]; ok {
				return nil
			}

			// Similarly, if the object isn't owned by `rp`, don't consider it
			// for deletion.
			if !isOwned {
				return nil
			}

			toDelete = append(toDelete, obj)

			return nil
		}); err != nil {
			return err
		}
	}

	log.Info(fmt.Sprintf("identified %d objects to gc", len(toDelete)))

	var errs []error
	for _, obj := range toDelete {
		if err := r.Client.Delete(ctx, obj); err != nil {
			errs = append(errs, errors.Wrapf(err, "gc'ing %T: %s", obj, obj.GetName()))
		}
	}

	return errors.Join(errs...)
}

func (r *RedpandaReconciler) reconcile(ctx context.Context, rp *v1alpha2.Redpanda) (*v1alpha2.Redpanda, error) {
	log := ctrl.LoggerFrom(ctx)
	log.WithName("RedpandaReconciler.reconcile")

	// pull our deployments and stateful sets
	redpandaStatefulSets, err := redpandaStatefulSetsForCluster(ctx, r.Client, rp)
	if err != nil {
		return rp, err
	}
	consoleDeployments, err := consoleDeploymentsForCluster(ctx, r.Client, rp)
	if err != nil {
		return rp, err
	}

	// Check if HelmRepository exists or create it
	if err := r.reconcileHelmRepository(ctx, rp); err != nil {
		return rp, err
	}

	if !ptr.Deref(rp.Status.HelmRepositoryReady, false) {
		// strip out all of the requeues since this will get requeued based on the Owns in the setup of the reconciler
		msgNotReady := fmt.Sprintf(resourceNotReadyStrFmt, resourceTypeHelmRepository, rp.Namespace, rp.GetHelmReleaseName())
		return v1alpha2.RedpandaNotReady(rp, "ArtifactFailed", msgNotReady), nil
	}

	// Check if HelmRelease exists or create it also
	if err := r.reconcileHelmRelease(ctx, rp); err != nil {
		return rp, err
	}

	if !ptr.Deref(rp.Status.HelmReleaseReady, false) {
		// strip out all of the requeues since this will get requeued based on the Owns in the setup of the reconciler
		msgNotReady := fmt.Sprintf(resourceNotReadyStrFmt, resourceTypeHelmRelease, rp.GetNamespace(), rp.GetHelmReleaseName())
		return v1alpha2.RedpandaNotReady(rp, "ArtifactFailed", msgNotReady), nil
	}

	if len(redpandaStatefulSets) == 0 {
		return v1alpha2.RedpandaNotReady(rp, "RedpandaPodsNotReady", "Redpanda StatefulSet not yet created"), nil
	}

	// check to make sure that our stateful set pods are all current
	if message, ready := checkStatefulSetStatus(redpandaStatefulSets); !ready {
		return v1alpha2.RedpandaNotReady(rp, "RedpandaPodsNotReady", message), nil
	}

	// check to make sure that our deployment pods are all current
	if message, ready := checkDeploymentsStatus(consoleDeployments); !ready {
		return v1alpha2.RedpandaNotReady(rp, "ConsolePodsNotReady", message), nil
	}

	// Once we know that STS Pods are up and running, make sure that we don't
	// need to perform a decommission.
	needsDecommission, err := r.needsDecommission(ctx, rp, redpandaStatefulSets)
	if err != nil {
		return rp, err
	}

	if needsDecommission {
		return v1alpha2.RedpandaNotReady(rp, "RedpandaPodsNotReady", "Cluster currently decommissioning dead nodes"), nil
	}

	return v1alpha2.RedpandaReady(rp), nil
}

func (r *RedpandaReconciler) needsDecommission(ctx context.Context, rp *v1alpha2.Redpanda, stses []*appsv1.StatefulSet) (bool, error) {
	client, err := r.ClientFactory.RedpandaAdminClient(ctx, rp)
	if err != nil {
		return false, err
	}

	health, err := client.GetHealthOverview(ctx)
	if err != nil {
		return false, errors.WithStack(err)
	}

	desiredReplicas := 0
	for _, sts := range stses {
		desiredReplicas += int(ptr.Deref(sts.Spec.Replicas, 0))
	}

	if len(health.AllNodes) == 0 || desiredReplicas == 0 {
		return false, nil
	}

	return len(health.AllNodes) > desiredReplicas, nil
}

func (r *RedpandaReconciler) reconcileHelmRelease(ctx context.Context, rp *v1alpha2.Redpanda) error {
	hr, err := r.createHelmReleaseFromTemplate(ctx, rp)
	if err != nil {
		return err
	}

	if err := r.apply(ctx, hr); err != nil {
		return err
	}

	isGenerationCurrent := hr.Generation == hr.Status.ObservedGeneration
	isStatusConditionReady := apimeta.IsStatusConditionTrue(hr.Status.Conditions, meta.ReadyCondition) || apimeta.IsStatusConditionTrue(hr.Status.Conditions, helmv2beta2.RemediatedCondition)

	// When UseFlux is false, we suspend the HelmRelease which completely
	// disables the controller. In such cases, we have to lie a bit to keep
	// everything else chugging along as expected.
	if hr.Spec.Suspend {
		isGenerationCurrent = true
		isStatusConditionReady = true
	}

	rp.Status.HelmRelease = hr.Name
	rp.Status.HelmReleaseReady = ptr.To(isGenerationCurrent && isStatusConditionReady)

	return nil
}

func (r *RedpandaReconciler) reconcileHelmRepository(ctx context.Context, rp *v1alpha2.Redpanda) error {
	repo := r.helmRepositoryFromTemplate(rp)

	if err := r.apply(ctx, repo); err != nil {
		return fmt.Errorf("applying HelmRepository: %w", err)
	}

	isGenerationCurrent := repo.Generation == repo.Status.ObservedGeneration
	isStatusConditionReady := apimeta.IsStatusConditionTrue(repo.Status.Conditions, meta.ReadyCondition)

	// When UseFlux is false, we suspend the HelmRepository which completely
	// disables the controller. In such cases, we have to lie a bit to keep
	// everything else chugging along as expected.
	if repo.Spec.Suspend {
		isGenerationCurrent = true
		isStatusConditionReady = true
	}

	rp.Status.HelmRepository = repo.Name
	rp.Status.HelmRepositoryReady = ptr.To(isStatusConditionReady && isGenerationCurrent)

	return nil
}

func (r *RedpandaReconciler) reconcileDelete(ctx context.Context, rp *v1alpha2.Redpanda) (ctrl.Result, error) {
	if err := r.deleteHelmRelease(ctx, rp); err != nil {
		return ctrl.Result{}, err
	}
	if controllerutil.ContainsFinalizer(rp, FinalizerKey) {
		controllerutil.RemoveFinalizer(rp, FinalizerKey)
		if err := r.Client.Update(ctx, rp); err != nil {
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

func (r *RedpandaReconciler) deleteHelmRelease(ctx context.Context, rp *v1alpha2.Redpanda) error {
	if rp.Status.HelmRelease == "" {
		return nil
	}

	var hr helmv2beta2.HelmRelease
	hrName := rp.Status.GetHelmRelease()
	err := r.Client.Get(ctx, types.NamespacedName{Namespace: rp.Namespace, Name: hrName}, &hr)
	if err != nil {
		if apierrors.IsNotFound(err) {
			rp.Status.HelmRelease = ""
			rp.Status.HelmRepository = ""
			return nil
		}
		return fmt.Errorf("failed to get HelmRelease '%s': %w", rp.Status.HelmRelease, err)
	}

	if err := r.Client.Delete(ctx, &hr, client.PropagationPolicy(metav1.DeletePropagationForeground)); err != nil {
		return fmt.Errorf("deleting helm release connected with Redpanda (%s): %w", rp.Name, err)
	}

	return errWaitForReleaseDeletion
}

func (r *RedpandaReconciler) createHelmReleaseFromTemplate(ctx context.Context, rp *v1alpha2.Redpanda) (*helmv2beta2.HelmRelease, error) {
	log := ctrl.LoggerFrom(ctx).WithName("RedpandaReconciler.createHelmReleaseFromTemplate")

	values, err := rp.ValuesJSON()
	if err != nil {
		return nil, fmt.Errorf("could not parse clusterSpec to json: %w", err)
	}

	log.V(logger.DebugLevel).Info("helm release values", "raw-values", string(values.Raw))

	hasher := sha256.New()
	hasher.Write(values.Raw)
	sha := base64.URLEncoding.EncodeToString(hasher.Sum(nil))
	// TODO possibly add the SHA to the status
	log.Info(fmt.Sprintf("SHA of values file to use: %s", sha))

	timeout := rp.Spec.ChartRef.Timeout
	if timeout == nil {
		timeout = &metav1.Duration{Duration: 15 * time.Minute}
	}

	chartVersion := rp.Spec.ChartRef.ChartVersion
	if chartVersion == "" {
		chartVersion = redpanda.Chart.Metadata().Version
	}

	upgrade := &helmv2beta2.Upgrade{
		// we skip waiting since relying on the Helm release process
		// to actually happen means that we block running any sort
		// of pending upgrades while we are attempting the upgrade job.
		DisableWait:        true,
		DisableWaitForJobs: true,
	}

	helmUpgrade := rp.Spec.ChartRef.Upgrade
	if rp.Spec.ChartRef.Upgrade != nil {
		if helmUpgrade.Force != nil {
			upgrade.Force = ptr.Deref(helmUpgrade.Force, false)
		}
		if helmUpgrade.CleanupOnFail != nil {
			upgrade.CleanupOnFail = ptr.Deref(helmUpgrade.CleanupOnFail, false)
		}
		if helmUpgrade.PreserveValues != nil {
			upgrade.PreserveValues = ptr.Deref(helmUpgrade.PreserveValues, false)
		}
		if helmUpgrade.Remediation != nil {
			upgrade.Remediation = helmUpgrade.Remediation
		}
	}

	return &helmv2beta2.HelmRelease{
		ObjectMeta: metav1.ObjectMeta{
			Name:            rp.GetHelmReleaseName(),
			Namespace:       rp.Namespace,
			OwnerReferences: []metav1.OwnerReference{rp.OwnerShipRefObj()},
		},
		Spec: helmv2beta2.HelmReleaseSpec{
			Suspend: !ptr.Deref(rp.Spec.ChartRef.UseFlux, true),
			Chart: helmv2beta2.HelmChartTemplate{
				Spec: helmv2beta2.HelmChartTemplateSpec{
					Chart:    "redpanda",
					Version:  chartVersion,
					Interval: &metav1.Duration{Duration: 1 * time.Minute},
					SourceRef: helmv2beta2.CrossNamespaceObjectReference{
						Kind:      "HelmRepository",
						Name:      rp.GetHelmRepositoryName(),
						Namespace: rp.Namespace,
					},
				},
			},
			Values:   values,
			Interval: metav1.Duration{Duration: 30 * time.Second},
			Timeout:  timeout,
			Upgrade:  upgrade,
		},
	}, nil
}

func (r *RedpandaReconciler) helmRepositoryFromTemplate(rp *v1alpha2.Redpanda) *sourcev1.HelmRepository {
	return &sourcev1.HelmRepository{
		ObjectMeta: metav1.ObjectMeta{
			Name:            rp.GetHelmRepositoryName(),
			Namespace:       rp.Namespace,
			OwnerReferences: []metav1.OwnerReference{rp.OwnerShipRefObj()},
		},
		Spec: sourcev1.HelmRepositorySpec{
			Suspend:  !ptr.Deref(rp.Spec.ChartRef.UseFlux, true),
			Interval: metav1.Duration{Duration: 30 * time.Second},
			URL:      v1alpha2.RedpandaChartRepository,
		},
	}
}

func (r *RedpandaReconciler) patchRedpandaStatus(ctx context.Context, rp *v1alpha2.Redpanda) error {
	key := client.ObjectKeyFromObject(rp)
	latest := &v1alpha2.Redpanda{}
	if err := r.Client.Get(ctx, key, latest); err != nil {
		return err
	}
	return r.Client.Status().Patch(ctx, rp, client.MergeFrom(latest))
}

func (r *RedpandaReconciler) apply(ctx context.Context, obj client.Object) error {
	gvk, err := r.Client.GroupVersionKindFor(obj)
	if err != nil {
		return err
	}

	obj.SetManagedFields(nil)
	obj.GetObjectKind().SetGroupVersionKind(gvk)

	return errors.WithStack(r.Client.Patch(ctx, obj, client.Apply, client.ForceOwnership, client.FieldOwner("redpanda-operator")))
}

func isRedpandaManaged(ctx context.Context, redpandaCluster *v1alpha2.Redpanda) bool {
	log := ctrl.LoggerFrom(ctx).WithName("RedpandaReconciler.isRedpandaManaged")

	managedAnnotationKey := v1alpha2.GroupVersion.Group + managedPath
	if managed, exists := redpandaCluster.Annotations[managedAnnotationKey]; exists && managed == NotManaged {
		log.Info(fmt.Sprintf("management is disabled; to enable it, change the '%s' annotation to true or remove it", managedAnnotationKey))
		return false
	}
	return true
}

func checkDeploymentsStatus(deployments []*appsv1.Deployment) (string, bool) {
	return checkReplicasForList(func(o *appsv1.Deployment) (int32, int32, int32, int32) {
		return o.Status.UpdatedReplicas, o.Status.AvailableReplicas, o.Status.ReadyReplicas, ptr.Deref(o.Spec.Replicas, 0)
	}, deployments, "Deployment")
}

func checkStatefulSetStatus(ss []*appsv1.StatefulSet) (string, bool) {
	return checkReplicasForList(func(o *appsv1.StatefulSet) (int32, int32, int32, int32) {
		return o.Status.UpdatedReplicas, o.Status.AvailableReplicas, o.Status.ReadyReplicas, ptr.Deref(o.Spec.Replicas, 0)
	}, ss, "StatefulSet")
}

type replicasExtractor[T client.Object] func(o T) (updated, available, ready, total int32)

func checkReplicasForList[T client.Object](fn replicasExtractor[T], list []T, resource string) (string, bool) {
	var notReady sort.StringSlice
	for _, item := range list {
		updated, available, ready, total := fn(item)

		if updated != total || available != total || ready != total {
			name := client.ObjectKeyFromObject(item).String()
			item := fmt.Sprintf("%q (updated/available/ready/total: %d/%d/%d/%d)", name, updated, available, ready, total)
			notReady = append(notReady, item)
		}
	}
	if len(notReady) > 0 {
		notReady.Sort()

		return fmt.Sprintf("Not all %s replicas updated, available, and ready for [%s]", resource, strings.Join(notReady, "; ")), false
	}
	return "", true
}

func allListTypes(c client.Client) ([]client.ObjectList, error) {
	// TODO: iterators would be really cool here.
	var types []client.ObjectList
	for _, t := range redpanda.Types() {
		gvk, err := c.GroupVersionKindFor(t)
		if err != nil {
			return nil, err
		}

		gvk.Kind += "List"

		list, err := c.Scheme().New(gvk)
		if err != nil {
			return nil, err
		}

		types = append(types, list.(client.ObjectList))
	}
	return types, nil
}
