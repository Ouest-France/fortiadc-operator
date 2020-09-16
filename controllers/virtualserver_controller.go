/*


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

package controllers

import (
	"context"
	"fmt"
	"time"

	"github.com/Ouest-France/gofortiadc"
	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	api "github.com/Ouest-France/fortiadc-operator/api/v1"
	fortiadcv1 "github.com/Ouest-France/fortiadc-operator/api/v1"
)

// VirtualServerReconciler reconciles a VirtualServer object
type VirtualServerReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=fortiadc.ouest-france.fr,resources=virtualservers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=fortiadc.ouest-france.fr,resources=virtualservers/status,verbs=get;update;patch

func (r *VirtualServerReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("virtualserver", req.NamespacedName)

	// Create FortiADC client
	fortiClient, err := NewFortiClient()
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to create Fortiadc client: %w", err)
	}

	// Fetch VirtualServer
	var virtualserver api.VirtualServer
	err = r.Get(ctx, req.NamespacedName, &virtualserver)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// examine DeletionTimestamp to determine if object is under deletion
	finalizerName := "fortiadc.ouest-france.fr/virtualserver"
	if virtualserver.ObjectMeta.DeletionTimestamp.IsZero() {

		log.Info("Set finalizer")

		if !containsString(virtualserver.ObjectMeta.Finalizers, finalizerName) {
			virtualserver.ObjectMeta.Finalizers = append(virtualserver.ObjectMeta.Finalizers, finalizerName)
			if err := r.Update(context.Background(), &virtualserver); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to add finalizer: %w", err)
			}
		}
	} else {

		log.Info("Execute finalizer")

		// The object is being deleted
		if containsString(virtualserver.ObjectMeta.Finalizers, finalizerName) {

			if err := r.DeleteVirtualServer(ctx, fortiClient, virtualserver); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to delete forti virtual server on kube virtual server delete: %w", err)
			}

			if err := r.DeletePool(ctx, req.NamespacedName); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to delete pool on virtual server delete: %w", err)
			}

			virtualserver.ObjectMeta.Finalizers = removeString(virtualserver.ObjectMeta.Finalizers, finalizerName)
			if err := r.Update(context.Background(), &virtualserver); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to remove finalizer: %w", err)
			}
		}

		return ctrl.Result{}, nil
	}

	// Create/Update Pool
	log.Info("Create or Update Pool", "service", req.Name)
	err = r.CreateOrUpdatePool(ctx, virtualserver)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update/create pool: %w", err)
	}

	// Fetch Pool
	var pool api.Pool
	err = r.Get(ctx, req.NamespacedName, &pool)
	if client.IgnoreNotFound(err) != nil {
		return ctrl.Result{}, fmt.Errorf("failed to fetch pool: %w", err)
	}
	if apierrors.IsNotFound(err) {
		return ctrl.Result{}, fmt.Errorf("pool must be created before virtual server creation: %w", err)
	}

	// Create/Update Virtual Server
	log.Info("Create or Update VirtualServer", "service", req.Name)
	err = r.CreateOrUpdateVirtualServer(ctx, fortiClient, virtualserver, pool)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update/create virtual server: %w", err)
	}

	return ctrl.Result{RequeueAfter: time.Second * 300}, nil
}

func (r *VirtualServerReconciler) DeletePool(ctx context.Context, namespacedName types.NamespacedName) error {

	// Fetch Pool
	var pool api.Pool
	err := r.Get(ctx, namespacedName, &pool)
	if client.IgnoreNotFound(err) != nil {
		return fmt.Errorf("Failed to fetch kube Pool")
	}

	// Pool doesn't exists
	if apierrors.IsNotFound(err) {
		return nil
	}

	// Delete Pool
	err = r.Delete(ctx, &pool)
	if err != nil {
		return fmt.Errorf("failed to delete pool: %w", err)
	}

	return nil
}

func (r *VirtualServerReconciler) CreateOrUpdatePool(ctx context.Context, virtualserver api.VirtualServer) error {

	// Fetch Pool
	var pool api.Pool
	err := r.Get(ctx, types.NamespacedName{Name: virtualserver.Name, Namespace: virtualserver.Namespace}, &pool)
	if client.IgnoreNotFound(err) != nil {
		return fmt.Errorf("failed to fetch kube Pool: %w", err)
	}

	// Pool doesn't exists
	if apierrors.IsNotFound(err) {
		err = r.CreatePool(ctx, virtualserver)
		if err != nil {
			return fmt.Errorf("failed to create kube Pool: %w", err)
		}
		return nil
	}

	// Update Pool
	err = r.UpdatePool(ctx, pool, virtualserver)
	if err != nil {
		return fmt.Errorf("failed to update kube Pool: %w", err)
	}

	return nil
}

func (r *VirtualServerReconciler) CreatePool(ctx context.Context, virtualserver api.VirtualServer) error {

	// Create Pool
	pool := &api.Pool{
		ObjectMeta: metav1.ObjectMeta{
			Labels:      make(map[string]string),
			Annotations: make(map[string]string),
			Name:        virtualserver.Name,
			Namespace:   virtualserver.Namespace,
		},
		Spec: api.PoolSpec{
			TargetPort: virtualserver.Spec.TargetPort,
			Name:       virtualserver.Spec.Name,
		},
	}

	// Create
	err := r.Create(ctx, pool)
	if err != nil {
		return fmt.Errorf("failed to create kube Pool: %w", err)
	}

	// Set owner
	err = ctrl.SetControllerReference(&virtualserver, pool, r.Scheme)
	if err != nil {
		return fmt.Errorf("failed to set controller reference on kube Pool: %w", err)
	}

	return nil
}

func (r *VirtualServerReconciler) UpdatePool(ctx context.Context, pool api.Pool, virtualserver api.VirtualServer) error {

	// Update fields
	pool.Spec.TargetPort = virtualserver.Spec.TargetPort

	// Update
	err := r.Update(ctx, &pool)
	if err != nil {
		return fmt.Errorf("failed to update kube Pool: %w", err)
	}

	// Set owner
	err = ctrl.SetControllerReference(&virtualserver, &pool, r.Scheme)
	if err != nil {
		return fmt.Errorf("failed to set controller reference on kube Pool: %w", err)
	}

	return nil
}

func (r *VirtualServerReconciler) DeleteVirtualServer(ctx context.Context, fortiClient gofortiadc.Client, kubeVirtualServer api.VirtualServer) error {

	// Get forti Virtual Servers
	fortiVirtualServers, err := fortiClient.LoadbalanceGetVirtualServers()
	if err != nil {
		return fmt.Errorf("failed to list forti virtual servers: %w", err)
	}
	fortiVirtualServersMap := map[string]gofortiadc.LoadbalanceVirtualServer{}
	for _, vs := range fortiVirtualServers {
		fortiVirtualServersMap[vs.Mkey] = vs
	}

	// Exit if forti virtual server doesn't exist
	_, ok := fortiVirtualServersMap[kubeVirtualServer.Spec.Name]
	if !ok {
		return nil
	}

	// Delete Virtual Server
	err = fortiClient.LoadbalanceDeleteVirtualServer(kubeVirtualServer.Spec.Name)
	if err != nil {
		return fmt.Errorf("failed to delete forti virtual server: %w", err)
	}

	return nil
}

func (r *VirtualServerReconciler) CreateOrUpdateVirtualServer(ctx context.Context, fortiClient gofortiadc.Client, kubeVirtualServer api.VirtualServer, kubePool api.Pool) error {

	// Get forti Virtual Servers
	fortiVirtualServers, err := fortiClient.LoadbalanceGetVirtualServers()
	if err != nil {
		return fmt.Errorf("failed to list forti virtual servers: %w", err)
	}
	fortiVirtualServersMap := map[string]gofortiadc.LoadbalanceVirtualServer{}
	for _, vs := range fortiVirtualServers {
		fortiVirtualServersMap[vs.Mkey] = vs
	}

	// Create if forti virtual server doesn't exist
	fortiVirtualServer, ok := fortiVirtualServersMap[kubeVirtualServer.Spec.Name]
	if !ok {
		err = r.CreateRealVirtualServer(ctx, fortiClient, kubeVirtualServer, kubePool)
		if err != nil {
			return fmt.Errorf("failed to create forti Virtual Server: %w", err)
		}
		return nil
	}

	// Update VirtualServer
	err = r.UpdateVirtualServer(ctx, fortiClient, kubeVirtualServer, fortiVirtualServer, kubePool)
	if err != nil {
		return fmt.Errorf("failed to update forti Virtual Server: %w", err)
	}

	return nil
}

func (r *VirtualServerReconciler) CreateRealVirtualServer(ctx context.Context, fortiClient gofortiadc.Client, kubeVirtualServer api.VirtualServer, kubePool api.Pool) error {

	// Create Virtual Server
	fortiVirtualServer := gofortiadc.LoadbalanceVirtualServer{
		Status:               "enable",
		Type:                 "l4-load-balance",
		AddrType:             "ipv4",
		Address:              kubeVirtualServer.Spec.IP,
		Address6:             "::",
		PacketFwdMethod:      "FullNAT",
		SourcePoolList:       "POOL_1",
		Port:                 kubeVirtualServer.Spec.Port,
		ConnectionLimit:      "0",
		ContentRouting:       "disable",
		ContentRoutingList:   "",
		ContentRewriting:     "disable",
		ContentRewritingList: "",
		Warmup:               "0",
		Warmrate:             "10",
		ConnectionRateLimit:  "0",
		Alone:                "enable",
		Mkey:                 kubeVirtualServer.Spec.Name,
		Interface:            "port1",
		Profile:              "LB_PROF_TCP",
		Method:               "LB_METHOD_ROUND_ROBIN",
		Pool:                 kubePool.Spec.Name,
		ClientSSLProfile:     "",
		HTTP2HTTPS:           "",
		Persistence:          "",
		ErrorMsg:             "Server-unavailable!",
		ErrorPage:            "",
		TrafficLog:           "disable",
	}
	err := fortiClient.LoadbalanceCreateVirtualServer(fortiVirtualServer)
	if err != nil {
		return fmt.Errorf("failed to create forti virtual server: %w", err)
	}

	return nil
}

func (r *VirtualServerReconciler) UpdateVirtualServer(ctx context.Context, fortiClient gofortiadc.Client, kubeVirtualServer api.VirtualServer, fortiVirtualServer gofortiadc.LoadbalanceVirtualServer, kubePool api.Pool) error {

	// Update Virtual Server
	fortiVirtualServer.Address = kubeVirtualServer.Spec.IP
	fortiVirtualServer.Port = kubeVirtualServer.Spec.Port
	fortiVirtualServer.Pool = kubePool.Name

	err := fortiClient.LoadbalanceUpdateVirtualServer(fortiVirtualServer)
	if err != nil {
		return fmt.Errorf("failed to update forti virtual server: %w", err)
	}

	return nil
}

func (r *VirtualServerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&fortiadcv1.VirtualServer{}).
		Complete(r)
}
