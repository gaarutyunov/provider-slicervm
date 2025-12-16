/*
Copyright 2025 The Crossplane Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package vm

import (
	"context"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/v2/pkg/controller"
	"github.com/crossplane/crossplane-runtime/v2/pkg/event"
	"github.com/crossplane/crossplane-runtime/v2/pkg/feature"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/ratelimiter"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"github.com/crossplane/crossplane-runtime/v2/pkg/statemetrics"
	"github.com/pkg/errors"
	sdk "github.com/slicervm/sdk"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apisv1alpha1 "github.com/gaarutyunov/provider-slicervm/apis/v1alpha1"
	"github.com/gaarutyunov/provider-slicervm/apis/vm/v1alpha1"
)

const (
	errNotVM        = "managed resource is not a VM custom resource"
	errTrackPCUsage = "cannot track ProviderConfig usage"
	errGetPC        = "cannot get ProviderConfig"
	errGetCPC       = "cannot get ClusterProviderConfig"
	errGetCreds     = "cannot get credentials"
	errNewClient    = "cannot create new Slicer client"
)

// SetupGated adds a controller that reconciles VM managed resources with safe-start support.
func SetupGated(mgr ctrl.Manager, o controller.Options) error {
	o.Gate.Register(func() {
		if err := Setup(mgr, o); err != nil {
			panic(errors.Wrap(err, "cannot setup VM controller"))
		}
	}, v1alpha1.VMGroupVersionKind)
	return nil
}

// Setup adds a controller that reconciles VM managed resources.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	name := managed.ControllerName(v1alpha1.VMGroupKind)

	opts := []managed.ReconcilerOption{
		managed.WithExternalConnector(&connector{
			kube:  mgr.GetClient(),
			usage: resource.NewProviderConfigUsageTracker(mgr.GetClient(), &apisv1alpha1.ProviderConfigUsage{}),
		}),
		managed.WithLogger(o.Logger.WithValues("controller", name)),
		managed.WithPollInterval(o.PollInterval),
		managed.WithRecorder(event.NewAPIRecorder(mgr.GetEventRecorderFor(name))),
	}

	if o.Features.Enabled(feature.EnableBetaManagementPolicies) {
		opts = append(opts, managed.WithManagementPolicies())
	}

	if o.Features.Enabled(feature.EnableAlphaChangeLogs) {
		opts = append(opts, managed.WithChangeLogger(o.ChangeLogOptions.ChangeLogger))
	}

	if o.MetricOptions != nil {
		opts = append(opts, managed.WithMetricRecorder(o.MetricOptions.MRMetrics))
	}

	if o.MetricOptions != nil && o.MetricOptions.MRStateMetrics != nil {
		stateMetricsRecorder := statemetrics.NewMRStateRecorder(
			mgr.GetClient(), o.Logger, o.MetricOptions.MRStateMetrics, &v1alpha1.VMList{}, o.MetricOptions.PollStateMetricInterval,
		)
		if err := mgr.Add(stateMetricsRecorder); err != nil {
			return errors.Wrap(err, "cannot register MR state metrics recorder for kind v1alpha1.VMList")
		}
	}

	r := managed.NewReconciler(mgr, resource.ManagedKind(v1alpha1.VMGroupVersionKind), opts...)

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(o.ForControllerRuntime()).
		WithEventFilter(resource.DesiredStateChanged()).
		For(&v1alpha1.VM{}).
		Complete(ratelimiter.NewReconciler(name, r, o.GlobalRateLimiter))
}

// connector produces an ExternalClient when its Connect method is called.
type connector struct {
	kube  client.Client
	usage *resource.ProviderConfigUsageTracker
}

// slicerConfig holds the configuration needed to create a Slicer client.
type slicerConfig struct {
	URL       string
	Token     string
	HostGroup string
}

// Connect produces an ExternalClient by getting credentials from the ProviderConfig.
func (c *connector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
	cr, ok := mg.(*v1alpha1.VM)
	if !ok {
		return nil, errors.New(errNotVM)
	}

	if err := c.usage.Track(ctx, cr); err != nil {
		return nil, errors.Wrap(err, errTrackPCUsage)
	}

	var cfg slicerConfig
	var cd apisv1alpha1.ProviderCredentials

	// Get ProviderConfigRef
	m := mg.(resource.ModernManaged)
	ref := m.GetProviderConfigReference()

	switch ref.Kind {
	case "ProviderConfig":
		pc := &apisv1alpha1.ProviderConfig{}
		if err := c.kube.Get(ctx, types.NamespacedName{Name: ref.Name, Namespace: m.GetNamespace()}, pc); err != nil {
			return nil, errors.Wrap(err, errGetPC)
		}
		cd = pc.Spec.Credentials
		cfg.URL = pc.Spec.URL
		cfg.HostGroup = pc.Spec.HostGroup
	case "ClusterProviderConfig":
		cpc := &apisv1alpha1.ClusterProviderConfig{}
		if err := c.kube.Get(ctx, types.NamespacedName{Name: ref.Name}, cpc); err != nil {
			return nil, errors.Wrap(err, errGetCPC)
		}
		cd = cpc.Spec.Credentials
		cfg.URL = cpc.Spec.URL
		cfg.HostGroup = cpc.Spec.HostGroup
	default:
		return nil, errors.Errorf("unsupported provider config kind: %s", ref.Kind)
	}

	// Set defaults
	if cfg.URL == "" {
		cfg.URL = "http://127.0.0.1:8080"
	}
	if cfg.HostGroup == "" {
		cfg.HostGroup = "api"
	}

	// Get credentials
	data, err := resource.CommonCredentialExtractor(ctx, cd.Source, c.kube, cd.CommonCredentialSelectors)
	if err != nil {
		return nil, errors.Wrap(err, errGetCreds)
	}
	cfg.Token = string(data)

	// Create Slicer client
	slicerClient := sdk.NewSlicerClient(cfg.URL, cfg.Token, "provider-slicervm/1.0", nil)

	return &external{
		client:    slicerClient,
		hostGroup: cfg.HostGroup,
	}, nil
}

// external observes, creates, updates, or deletes VMs using the Slicer SDK.
type external struct {
	client    *sdk.SlicerClient
	hostGroup string
}

func (e *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	cr, ok := mg.(*v1alpha1.VM)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotVM)
	}

	// If marked for deletion, report as non-existent to trigger Delete
	if meta.WasDeleted(mg) {
		return managed.ExternalObservation{ResourceExists: false}, nil
	}

	// Get external name (hostname)
	externalName := meta.GetExternalName(cr)
	if externalName == "" {
		return managed.ExternalObservation{ResourceExists: false}, nil
	}

	// Get host group
	hostGroup := cr.Spec.ForProvider.HostGroup
	if hostGroup == "" {
		hostGroup = e.hostGroup
	}

	// List VMs in the host group and find our VM
	nodes, err := e.client.GetHostGroupNodes(ctx, hostGroup)
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, "cannot list VMs")
	}

	var found *sdk.SlicerNode
	for i := range nodes {
		if nodes[i].Hostname == externalName {
			found = &nodes[i]
			break
		}
	}

	if found == nil {
		return managed.ExternalObservation{ResourceExists: false}, nil
	}

	// Update observed state
	cr.Status.AtProvider.Hostname = found.Hostname
	cr.Status.AtProvider.IP = found.IP
	cr.Status.AtProvider.CreatedAt = found.CreatedAt.String()
	cr.Status.AtProvider.State = "running"

	cr.SetConditions(xpv1.Available())

	return managed.ExternalObservation{
		ResourceExists:    true,
		ResourceUpToDate:  true,
		ConnectionDetails: managed.ConnectionDetails{},
	}, nil
}

func (e *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mg.(*v1alpha1.VM)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotVM)
	}

	cr.SetConditions(xpv1.Creating())

	// Get host group
	hostGroup := cr.Spec.ForProvider.HostGroup
	if hostGroup == "" {
		hostGroup = e.hostGroup
	}

	// Build request
	req := sdk.SlicerCreateNodeRequest{
		RamGB:    cr.Spec.ForProvider.RAMGB,
		CPUs:     cr.Spec.ForProvider.CPUs,
		Userdata: cr.Spec.ForProvider.Userdata,
	}

	if req.RamGB == 0 {
		req.RamGB = 4
	}
	if req.CPUs == 0 {
		req.CPUs = 2
	}

	if len(cr.Spec.ForProvider.SSHKeys) > 0 {
		req.SSHKeys = cr.Spec.ForProvider.SSHKeys
	}

	if cr.Spec.ForProvider.ImportUser != "" {
		req.ImportUser = cr.Spec.ForProvider.ImportUser
	}

	if len(cr.Spec.ForProvider.Tags) > 0 {
		req.Tags = cr.Spec.ForProvider.Tags
	}

	// Create VM
	resp, err := e.client.CreateNode(ctx, hostGroup, req)
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, "cannot create VM")
	}

	// Set external name to hostname
	meta.SetExternalName(cr, resp.Hostname)

	// Update status
	cr.Status.AtProvider.Hostname = resp.Hostname
	cr.Status.AtProvider.IP = resp.IP
	cr.Status.AtProvider.CreatedAt = resp.CreatedAt.String()

	return managed.ExternalCreation{
		ConnectionDetails: managed.ConnectionDetails{
			"hostname": []byte(resp.Hostname),
			"ip":       []byte(resp.IP),
		},
	}, nil
}

func (e *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	// Slicer VMs cannot be updated in place, only recreated
	// Return without error - Observe will handle the state
	return managed.ExternalUpdate{}, nil
}

func (e *external) Delete(ctx context.Context, mg resource.Managed) (managed.ExternalDelete, error) {
	cr, ok := mg.(*v1alpha1.VM)
	if !ok {
		return managed.ExternalDelete{}, errors.New(errNotVM)
	}

	cr.SetConditions(xpv1.Deleting())

	// Get external name (hostname)
	externalName := meta.GetExternalName(cr)
	if externalName == "" {
		return managed.ExternalDelete{}, nil
	}

	// Get host group
	hostGroup := cr.Spec.ForProvider.HostGroup
	if hostGroup == "" {
		hostGroup = e.hostGroup
	}

	// Delete VM
	_, err := e.client.DeleteVM(ctx, hostGroup, externalName)
	if err != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, "cannot delete VM")
	}

	return managed.ExternalDelete{}, nil
}

func (e *external) Disconnect(ctx context.Context) error {
	return nil
}
