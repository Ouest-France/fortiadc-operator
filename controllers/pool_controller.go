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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	api "github.com/Ouest-France/fortiadc-operator/api/v1"
	fortiadcv1 "github.com/Ouest-France/fortiadc-operator/api/v1"
)

// PoolReconciler reconciles a Pool object
type PoolReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=fortiadc.ouest-france.fr,resources=pools,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=fortiadc.ouest-france.fr,resources=pools/status,verbs=get;update;patch

func (r *PoolReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("pool", req.NamespacedName)

	// Create FortiADC client
	fortiClient, err := NewFortiClient()
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to create Fortiadc client: %w", err)
	}

	// Fetch Pool
	var pool api.Pool
	err = r.Get(ctx, req.NamespacedName, &pool)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// examine DeletionTimestamp to determine if object is under deletion
	finalizerName := "fortiadc.ouest-france.fr/pool"
	if pool.ObjectMeta.DeletionTimestamp.IsZero() {

		log.Info("Set finalizer")

		if !containsString(pool.ObjectMeta.Finalizers, finalizerName) {
			pool.ObjectMeta.Finalizers = append(pool.ObjectMeta.Finalizers, finalizerName)
			if err := r.Update(context.Background(), &pool); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to add finalizer: %w", err)
			}
		}
	} else {

		log.Info("Execute finalizer")

		// The object is being deleted
		if containsString(pool.ObjectMeta.Finalizers, finalizerName) {

			if err := r.DeleteRealServerPool(ctx, fortiClient, pool); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to delete forti real server pool on kube pool delete: %w", err)
			}

			pool.ObjectMeta.Finalizers = removeString(pool.ObjectMeta.Finalizers, finalizerName)
			if err := r.Update(context.Background(), &pool); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to remove finalizer: %w", err)
			}
		}

		return ctrl.Result{}, nil
	}

	// Create/Update Real Server Pool
	log.Info("Create or Update Real Server Pool", "pool", pool.Name)
	err = r.CreateOrUpdateRealServerPool(ctx, fortiClient, pool)
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: time.Second * 300}, nil
}

func (r *PoolReconciler) DeleteRealServerPool(ctx context.Context, fortiClient gofortiadc.Client, kubePool api.Pool) error {

	// Get forti Real Server Pools
	fortiPools, err := fortiClient.LoadbalanceGetPools()
	if err != nil {
		return fmt.Errorf("failed to list forti real server pools: %w", err)
	}
	fortiPoolsMap := map[string]gofortiadc.LoadbalancePool{}
	for _, p := range fortiPools {
		fortiPoolsMap[p.Mkey] = p
	}

	// Exit if forti real server pool doesn't exist
	_, ok := fortiPoolsMap[kubePool.Spec.Name]
	if !ok {
		return nil
	}

	// Delete Real Server
	err = fortiClient.LoadbalanceDeletePool(kubePool.Spec.Name)
	if err != nil {
		return fmt.Errorf("failed to delete forti real server pool: %w", err)
	}

	return nil
}

func (r *PoolReconciler) CreateOrUpdateRealServerPool(ctx context.Context, fortiClient gofortiadc.Client, kubePool api.Pool) error {

	// Get forti Real Server Pools
	fortiPools, err := fortiClient.LoadbalanceGetPools()
	if err != nil {
		return fmt.Errorf("failed to list forti real server pools: %w", err)
	}
	fortiPoolsMap := map[string]gofortiadc.LoadbalancePool{}
	for _, p := range fortiPools {
		fortiPoolsMap[p.Mkey] = p
	}

	// Create if forti real server pool doesn't exist
	fortiPool, ok := fortiPoolsMap[kubePool.Spec.Name]
	if !ok {
		err = r.CreateRealServerPool(ctx, fortiClient, kubePool)
		if err != nil {
			return fmt.Errorf("failed to create forti Real Server Pool: %w", err)
		}
		return nil
	}

	// Update RealServerPool
	err = r.UpdateRealServerPool(ctx, fortiClient, kubePool, fortiPool)
	if err != nil {
		return fmt.Errorf("failed to update forti Real Server Pool: %w", err)
	}

	return nil
}

func (r *PoolReconciler) CreateRealServerPool(ctx context.Context, fortiClient gofortiadc.Client, kubePool api.Pool) error {

	// Create Real Server Pool
	fortiPool := gofortiadc.LoadbalancePool{
		Mkey:                    kubePool.Spec.Name,
		PoolType:                "ipv4",
		HealthCheckRelationship: "AND",
		RsProfile:               "NONE",
	}
	err := fortiClient.LoadbalanceCreatePool(fortiPool)
	if err != nil {
		return fmt.Errorf("failed to create forti real server pool: %w", err)
	}

	err = r.SyncRealServerPoolMembers(ctx, fortiClient, kubePool, fortiPool)
	if err != nil {
		return fmt.Errorf("failed to sync forti real server pool members: %w", err)
	}

	return nil
}

func (r *PoolReconciler) UpdateRealServerPool(ctx context.Context, fortiClient gofortiadc.Client, kubePool api.Pool, fortiPool gofortiadc.LoadbalancePool) error {

	// Update RealServer
	err := fortiClient.LoadbalanceUpdatePool(fortiPool)
	if err != nil {
		return fmt.Errorf("failed to update forti real server pool: %w", err)
	}

	err = r.SyncRealServerPoolMembers(ctx, fortiClient, kubePool, fortiPool)
	if err != nil {
		return fmt.Errorf("failed to sync forti real server pool members: %w", err)
	}

	return nil
}

func (r *PoolReconciler) SyncRealServerPoolMembers(ctx context.Context, fortiClient gofortiadc.Client, kubePool api.Pool, fortiPool gofortiadc.LoadbalancePool) error {

	// Fetch nodes
	var nodes corev1.NodeList
	err := r.List(ctx, &nodes)
	if err != nil {
		return fmt.Errorf("failed to fetch Nodes: %w", err)
	}
	nodesMap := map[string]corev1.Node{}
	for _, node := range nodes.Items {
		nodesMap[node.Name] = node
	}

	// Fetch forti pool members
	fortiPoolMembers, err := fortiClient.LoadbalanceGetPoolMembers(fortiPool.Mkey)
	if err != nil {
		return fmt.Errorf("failed to list forti real server pool members: %w", err)
	}
	fortiPoolMembersMap := map[string]gofortiadc.LoadbalancePoolMember{}
	for _, m := range fortiPoolMembers {
		fortiPoolMembersMap[m.RealServerID] = m
	}

	// Create/Update members
	for _, node := range nodes.Items {
		member, ok := fortiPoolMembersMap[node.Name]
		if !ok {
			log.Log.Info("create real server pool member", "member", node.Name)
			err = r.CreateRealServerPoolMember(ctx, fortiClient, kubePool, fortiPool, node)
			if err != nil {
				return fmt.Errorf("failed to create/update forti Pool member: %w", err)
			}
		} else {
			log.Log.Info("update real server pool member", "member", node.Name)
			err = r.UpdateRealServerPoolMember(ctx, fortiClient, kubePool, fortiPool, node, member)
			if err != nil {
				return fmt.Errorf("failed to create/update forti Pool member: %w", err)
			}
		}
	}

	// Fetch updated forti pool members
	fortiPoolMembers, err = fortiClient.LoadbalanceGetPoolMembers(fortiPool.Mkey)
	if err != nil {
		return fmt.Errorf("failed to list forti real server pool members: %w", err)
	}
	fortiPoolMembersMap = map[string]gofortiadc.LoadbalancePoolMember{}
	for _, m := range fortiPoolMembers {
		fortiPoolMembersMap[m.RealServerID] = m
	}

	// Delete
	for _, fortiPoolMember := range fortiPoolMembers {
		_, ok := nodesMap[fortiPoolMember.RealServerID]
		if !ok {
			log.Log.Info("delete real server pool member", "member", fortiPoolMember.RealServerID)
			err = r.DeleteRealServerPoolMember(ctx, fortiClient, kubePool, fortiPool, fortiPoolMember)
			if err != nil {
				return fmt.Errorf("failed to delete forti Pool member: %w", err)
			}
		}
	}

	return nil
}

func (r *PoolReconciler) CreateRealServerPoolMember(ctx context.Context, fortiClient gofortiadc.Client, kubePool api.Pool, fortiPool gofortiadc.LoadbalancePool, node corev1.Node) error {

	// Create Real Server Pool Member
	err := fortiClient.LoadbalanceCreatePoolMember(fortiPool.Mkey, gofortiadc.LoadbalancePoolMember{
		Status:                   "enabled",
		Port:                     kubePool.Spec.TargetPort,
		RealServerID:             node.Name,
		Weight:                   "1",
		Warmup:                   "0",
		Connlimit:                "0",
		Recover:                  "0",
		Warmrate:                 "100",
		ConnectionRateLimit:      "0",
		Cookie:                   "",
		Backup:                   "disable",
		HealthCheckInherit:       "enable",
		MHealthCheck:             "disable",
		MHealthCheckRelationship: "AND",
		MHealthCheckList:         "",
		MysqlGroupID:             "0",
		MysqlReadOnly:            "disable",
		RsProfileInherit:         "enable",
		Ssl:                      "disable",
	})
	if err != nil {
		return fmt.Errorf("failed to create forti real server pool member %q: %w", node.Name, err)
	}

	return nil
}

func (r *PoolReconciler) UpdateRealServerPoolMember(ctx context.Context, fortiClient gofortiadc.Client, kubePool api.Pool, fortiPool gofortiadc.LoadbalancePool, node corev1.Node, fortiMember gofortiadc.LoadbalancePoolMember) error {

	// Update Real Server Pool Member
	fortiMember.Status = "enabled"
	fortiMember.Port = kubePool.Spec.TargetPort

	err := fortiClient.LoadbalanceUpdatePoolMember(fortiPool.Mkey, fortiMember.Mkey, fortiMember)
	if err != nil {
		return fmt.Errorf("failed to update forti real server pool member: %w", err)
	}

	return nil
}

func (r *PoolReconciler) DeleteRealServerPoolMember(ctx context.Context, fortiClient gofortiadc.Client, kubePool api.Pool, fortiPool gofortiadc.LoadbalancePool, fortiMember gofortiadc.LoadbalancePoolMember) error {

	// Delete Real Server Pool Member
	err := fortiClient.LoadbalanceDeletePoolMember(fortiPool.Mkey, fortiMember.Mkey)
	if err != nil {
		return fmt.Errorf("failed to delete forti real server pool member: %w", err)
	}

	return nil
}

func (r *PoolReconciler) SetupWithManager(mgr ctrl.Manager) error {

	// Create a reconcile request for each pool on nodes events
	nodeHandler := handler.ToRequestsFunc(
		func(a handler.MapObject) []reconcile.Request {

			var requests []reconcile.Request

			// Fetch pools
			var pools api.PoolList
			err := r.List(context.Background(), &pools)
			if err != nil {
				return requests
			}

			// Add request for each pool
			for _, pool := range pools.Items {
				requests = append(requests, reconcile.Request{NamespacedName: types.NamespacedName{Name: pool.Name, Namespace: pool.Namespace}})
			}

			return requests
		})

	return ctrl.NewControllerManagedBy(mgr).
		For(&fortiadcv1.Pool{}).
		Watches(&source.Kind{Type: &corev1.Node{}}, &handler.EnqueueRequestsFromMapFunc{ToRequests: nodeHandler}).
		Complete(r)
}
