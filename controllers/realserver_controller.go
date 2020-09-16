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

	api "github.com/Ouest-France/fortiadc-operator/api/v1"
	"github.com/Ouest-France/gofortiadc"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	fortiadcv1 "github.com/Ouest-France/fortiadc-operator/api/v1"
)

// RealServerReconciler reconciles a RealServer object
type RealServerReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=fortiadc.ouest-france.fr,resources=realservers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=fortiadc.ouest-france.fr,resources=realservers/status,verbs=get;update;patch

func (r *RealServerReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("realserver", req.NamespacedName)

	// Create FortiADC client
	fortiClient, err := NewFortiClient()
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to create Fortiadc client: %w", err)
	}

	// Fetch RealServer
	var rs api.RealServer
	err = r.Get(ctx, req.NamespacedName, &rs)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// examine DeletionTimestamp to determine if object is under deletion
	finalizerName := "fortiadc.ouest-france.fr/realserver"
	if rs.ObjectMeta.DeletionTimestamp.IsZero() {

		log.Info("Set finalizer")

		if !containsString(rs.ObjectMeta.Finalizers, finalizerName) {
			rs.ObjectMeta.Finalizers = append(rs.ObjectMeta.Finalizers, finalizerName)
			if err := r.Update(context.Background(), &rs); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to add finalizer: %w", err)
			}
		}
	} else {

		log.Info("Execute finalizer")

		// The object is being deleted
		if containsString(rs.ObjectMeta.Finalizers, finalizerName) {

			if err := r.DeleteRealServer(ctx, fortiClient, rs); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to delete forti real server on kube real server delete: %w", err)
			}

			rs.ObjectMeta.Finalizers = removeString(rs.ObjectMeta.Finalizers, finalizerName)
			if err := r.Update(context.Background(), &rs); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to remove finalizer: %w", err)
			}
		}

		return ctrl.Result{}, nil
	}

	// Create/Update Real Server
	log.Info("Create or Update Real Server", "service", req.Name)
	err = r.CreateOrUpdateRealServer(ctx, fortiClient, rs)
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: time.Second * 300}, nil
}

func (r *RealServerReconciler) DeleteRealServer(ctx context.Context, fortiClient gofortiadc.Client, kubeRealServer api.RealServer) error {

	// Get forti Real Servers
	fortiRealServers, err := fortiClient.LoadbalanceGetRealServers()
	if err != nil {
		return fmt.Errorf("failed to list forti real servers: %w", err)
	}
	fortiRealServersMap := map[string]gofortiadc.LoadbalanceRealServer{}
	for _, rs := range fortiRealServers {
		fortiRealServersMap[rs.Mkey] = rs
	}

	// Exit if forti real server doesn't exist
	_, ok := fortiRealServersMap[kubeRealServer.Name]
	if !ok {
		return nil
	}

	// Delete Real Server
	err = fortiClient.LoadbalanceDeleteRealServer(kubeRealServer.Name)
	if err != nil {
		return fmt.Errorf("failed to delete forti real server: %w", err)
	}

	return nil
}

func (r *RealServerReconciler) CreateOrUpdateRealServer(ctx context.Context, fortiClient gofortiadc.Client, kubeRealServer api.RealServer) error {

	// Get forti Real Servers
	fortiRealServers, err := fortiClient.LoadbalanceGetRealServers()
	if err != nil {
		return fmt.Errorf("failed to list forti real servers: %w", err)
	}
	fortiRealServersMap := map[string]gofortiadc.LoadbalanceRealServer{}
	for _, rs := range fortiRealServers {
		fortiRealServersMap[rs.Mkey] = rs
	}

	// Create if forti real server doesn't exist
	fortiRealServer, ok := fortiRealServersMap[kubeRealServer.Name]
	if !ok {
		err = r.CreateRealServer(ctx, fortiClient, kubeRealServer)
		if err != nil {
			return fmt.Errorf("failed to create forti Real Server: %w", err)
		}
		return nil
	}

	// Update RealServer
	err = r.UpdateRealServer(ctx, fortiClient, kubeRealServer, fortiRealServer)
	if err != nil {
		return fmt.Errorf("failed to update forti Real Server: %w", err)
	}

	return nil
}

func (r *RealServerReconciler) CreateRealServer(ctx context.Context, fortiClient gofortiadc.Client, kubeRealServer api.RealServer) error {

	// Create Real Server
	err := fortiClient.LoadbalanceCreateRealServer(gofortiadc.LoadbalanceRealServer{
		Status:   kubeRealServer.Spec.Status,
		Address:  kubeRealServer.Spec.Address,
		Address6: kubeRealServer.Spec.Address6,
		Mkey:     kubeRealServer.Name,
	})
	if err != nil {
		return fmt.Errorf("failed to create forti real server: %w", err)
	}

	return nil
}

func (r *RealServerReconciler) UpdateRealServer(ctx context.Context, fortiClient gofortiadc.Client, kubeRealServer api.RealServer, fortiRealServer gofortiadc.LoadbalanceRealServer) error {

	// Update RealServer
	fortiRealServer.Address = kubeRealServer.Spec.Address
	fortiRealServer.Address6 = kubeRealServer.Spec.Address6
	fortiRealServer.Status = kubeRealServer.Spec.Status

	err := fortiClient.LoadbalanceUpdateRealServer(fortiRealServer)
	if err != nil {
		return fmt.Errorf("failed to update forti real server: %w", err)
	}

	return nil
}

func (r *RealServerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&fortiadcv1.RealServer{}).
		Complete(r)
}
