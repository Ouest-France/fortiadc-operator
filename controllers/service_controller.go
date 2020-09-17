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
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// ServiceReconciler reconciles a Service object
type ServiceReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services/status,verbs=get;update;patch

func (r *ServiceReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {

	ctx := context.Background()
	log := r.Log.WithValues("service", req.NamespacedName)

	// Fetch Service
	var service corev1.Service
	err := r.Get(ctx, req.NamespacedName, &service)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// examine DeletionTimestamp to determine if object is under deletion
	finalizerName := "service.kubernetes.io/load-balancer-cleanup"
	if service.ObjectMeta.DeletionTimestamp.IsZero() {

		log.Info("Set finalizer")

		if !containsString(service.ObjectMeta.Finalizers, finalizerName) {
			service.ObjectMeta.Finalizers = append(service.ObjectMeta.Finalizers, finalizerName)
			if err := r.Update(context.Background(), &service); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to add finalizer: %w", err)
			}
		}
	} else {

		log.Info("Execute finalizer")

		// The object is being deleted
		if containsString(service.ObjectMeta.Finalizers, finalizerName) {

			if err := r.DeleteVirtualServer(ctx, req.NamespacedName); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to delete virtual server on service delete: %w", err)
			}

			service.ObjectMeta.Finalizers = removeString(service.ObjectMeta.Finalizers, finalizerName)
			if err := r.Update(context.Background(), &service); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to remove finalizer: %w", err)
			}
		}

		return ctrl.Result{}, nil
	}

	// Create/Update VirtualServer
	log.Info("Create or Update VirtualServer", "service", req.Name)
	err = r.CreateOrUpdateVirtualServer(ctx, service)
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: time.Second * 300}, nil
}

func (r *ServiceReconciler) DeleteVirtualServer(ctx context.Context, namespacedName types.NamespacedName) error {

	// Fetch VirtualServer
	var vs api.VirtualServer
	err := r.Get(ctx, namespacedName, &vs)
	if client.IgnoreNotFound(err) != nil {
		return fmt.Errorf("failed to fetch kube VirtualServer: %w", err)
	}

	// VirtualServer doesn't exists
	if apierrors.IsNotFound(err) {
		return nil
	}

	// Delete VirtualServer
	err = r.Delete(ctx, &vs)
	if err != nil {
		return fmt.Errorf("failed to delete kube Virtual Server: %w", err)
	}

	return nil
}

func (r *ServiceReconciler) CreateOrUpdateVirtualServer(ctx context.Context, service corev1.Service) error {

	// Check for virtualserver-name annotation
	virtualServerName, ok := service.Annotations["fortiadc.ouest-france.fr/virtualserver-name"]
	if !ok {
		log.Log.Info("no annotation fortiadc.ouest-france.fr/virtualserver-name found")
		return nil
	}

	// Fetch VirtualServer
	var vs api.VirtualServer
	err := r.Get(ctx, types.NamespacedName{Name: service.Name, Namespace: service.Namespace}, &vs)
	if client.IgnoreNotFound(err) != nil {
		return fmt.Errorf("failed to fetch kube VirtualServer: %w", err)
	}

	// VirtualServer doesn't exists
	if apierrors.IsNotFound(err) {
		err = r.CreateVirtualServer(ctx, virtualServerName, service)
		if err != nil {
			return fmt.Errorf("failed to create kube Virtual Server: %w", err)
		}
		return nil
	}

	// Update VirtualServer
	err = r.UpdateVirtualServer(ctx, vs, service)
	if err != nil {
		return fmt.Errorf("failed to update kube Virtual Server: %w", err)
	}

	return nil
}

func (r *ServiceReconciler) CreateVirtualServer(ctx context.Context, name string, service corev1.Service) error {

	// Create VirtualServer
	vs := &api.VirtualServer{
		ObjectMeta: metav1.ObjectMeta{
			Labels:      make(map[string]string),
			Annotations: make(map[string]string),
			Name:        service.Name,
			Namespace:   service.Namespace,
		},
		Spec: api.VirtualServerSpec{
			IP:         service.Spec.LoadBalancerIP,
			Port:       service.Spec.Ports[0].TargetPort.String(),
			TargetPort: fmt.Sprintf("%d ", service.Spec.Ports[0].NodePort),
			Name:       name,
		},
	}

	// Create
	err := r.Create(ctx, vs)
	if err != nil {
		return fmt.Errorf("failed to create kube Virtual Server: %w", err)
	}

	// Set owner
	err = ctrl.SetControllerReference(&service, vs, r.Scheme)
	if err != nil {
		return fmt.Errorf("failed to set controller reference on kube Virtual Server: %w", err)
	}

	// Set status
	service.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{corev1.LoadBalancerIngress{IP: service.Spec.LoadBalancerIP}}
	err = r.Update(ctx, &service)
	if err != nil {
		return fmt.Errorf("failed to update kube service status: %w", err)
	}

	return nil
}

func (r *ServiceReconciler) UpdateVirtualServer(ctx context.Context, vs api.VirtualServer, service corev1.Service) error {

	// Update fields
	vs.Spec.IP = service.Spec.LoadBalancerIP
	vs.Spec.Port = service.Spec.Ports[0].TargetPort.String()
	vs.Spec.TargetPort = fmt.Sprintf("%d ", service.Spec.Ports[0].NodePort)

	// Update
	err := r.Update(ctx, &vs)
	if err != nil {
		return fmt.Errorf("failed to update kube Virtual Server: %w", err)
	}

	// Set owner
	err = ctrl.SetControllerReference(&service, &vs, r.Scheme)
	if err != nil {
		return fmt.Errorf("failed to set controller reference on kube Virtual Server: %w", err)
	}

	// Set status
	service.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{corev1.LoadBalancerIngress{IP: service.Spec.LoadBalancerIP}}
	err = r.Update(ctx, &service)
	if err != nil {
		return fmt.Errorf("failed to update kube service status: %w", err)
	}

	return nil
}

func (r *ServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Service{}).
		Complete(r)
}
