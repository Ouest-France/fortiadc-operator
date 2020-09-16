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
	"errors"
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
)

// NodeReconciler reconciles a Node object
type NodeReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=core,resources=nodes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=nodes/status,verbs=get;update;patch

func (r *NodeReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("node", req.Name)

	// Fetch Node
	var node corev1.Node
	err := r.Get(ctx, req.NamespacedName, &node)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// examine DeletionTimestamp to determine if object is under deletion
	finalizerName := "fortiadc.ouest-france.fr/node"
	if node.ObjectMeta.DeletionTimestamp.IsZero() {

		log.Info("Set finalizer")

		if !containsString(node.ObjectMeta.Finalizers, finalizerName) {
			node.ObjectMeta.Finalizers = append(node.ObjectMeta.Finalizers, finalizerName)
			if err := r.Update(context.Background(), &node); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to add finalizer: %w", err)
			}
		}
	} else {

		log.Info("Execute finalizer")

		// The object is being deleted
		if containsString(node.ObjectMeta.Finalizers, finalizerName) {

			if err := r.DeleteRealServer(ctx, req.NamespacedName); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to delete real servers on node delete: %w", err)
			}

			node.ObjectMeta.Finalizers = removeString(node.ObjectMeta.Finalizers, finalizerName)
			if err := r.Update(context.Background(), &node); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to remove finalizer: %w", err)
			}
		}

		return ctrl.Result{}, nil
	}

	// Create/Update RealServer
	log.Info("Create or Update RealServer", "service", req.Name)
	err = r.CreateOrUpdateRealServer(ctx, node)
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: time.Second * 300}, nil
}

func (r *NodeReconciler) DeleteRealServer(ctx context.Context, namespacedName types.NamespacedName) error {

	// Fetch RealServer
	var rs api.RealServer
	err := r.Get(ctx, namespacedName, &rs)
	if client.IgnoreNotFound(err) != nil {
		return fmt.Errorf("failed to fetch kube RealServer: %w", err)
	}

	// RealServer doesn't exists
	if apierrors.IsNotFound(err) {
		return nil
	}

	// Delete RealServer
	err = r.Delete(ctx, &rs)
	if err != nil {
		return fmt.Errorf("failed to delete kube Real Server: %w", err)
	}

	return nil
}

func (r *NodeReconciler) CreateOrUpdateRealServer(ctx context.Context, node corev1.Node) error {

	// Fetch RealServer
	var rs api.RealServer
	err := r.Get(ctx, types.NamespacedName{Name: node.Name, Namespace: node.Namespace}, &rs)
	if client.IgnoreNotFound(err) != nil {
		return fmt.Errorf("failed to fetch kube RealServer: %w", err)
	}

	// RealServer doesn't exists
	if apierrors.IsNotFound(err) {
		err = r.CreateRealServer(ctx, node)
		if err != nil {
			return fmt.Errorf("failed to create kube Real Server: %w", err)
		}
		return nil
	}

	// Update RealServer
	err = r.UpdateRealServer(ctx, rs, node)
	if err != nil {
		return fmt.Errorf("failed to update kube Virtual Server: %w", err)
	}

	return nil
}

func (r *NodeReconciler) CreateRealServer(ctx context.Context, node corev1.Node) error {

	// Get node external IP
	ip, err := r.GetNodeExternalIP(node)
	if err != nil {
		return err
	}

	// Set status
	status := "enable"
	if node.Spec.Unschedulable {
		status = "disable"
	}
	for _, condition := range node.Status.Conditions {
		if condition.Type == "Ready" && condition.Status == "False" {
			status = "disable"
		}
	}

	// Create RealServer
	rs := &api.RealServer{
		ObjectMeta: metav1.ObjectMeta{
			Labels:      make(map[string]string),
			Annotations: make(map[string]string),
			Name:        node.Name,
		},
		Spec: api.RealServerSpec{
			Address:  ip,
			Address6: "::",
			Status:   status,
		},
	}

	// Create
	err = r.Create(ctx, rs)
	if err != nil {
		return fmt.Errorf("failed to create real server: %w", err)
	}

	return nil
}

func (r *NodeReconciler) UpdateRealServer(ctx context.Context, rs api.RealServer, node corev1.Node) error {

	// Get node external IP
	ip, err := r.GetNodeExternalIP(node)
	if err != nil {
		return err
	}

	// Set status
	status := "enable"
	if node.Spec.Unschedulable {
		status = "disable"
	}
	for _, condition := range node.Status.Conditions {
		if condition.Type == "Ready" && condition.Status == "False" {
			status = "disable"
		}
	}

	// Update fields
	rs.Spec.Address = ip
	rs.Spec.Address6 = "::"
	rs.Spec.Status = status

	// Update
	err = r.Update(ctx, &rs)
	if err != nil {
		return fmt.Errorf("failed to update real server: %w", err)
	}

	return nil
}

func (r *NodeReconciler) GetNodeExternalIP(node corev1.Node) (string, error) {

	for _, address := range node.Status.Addresses {
		if address.Type == corev1.NodeExternalIP {
			return address.Address, nil
		}
	}

	return "", errors.New("no external IP found")
}

func (r *NodeReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Node{}).
		Complete(r)
}
