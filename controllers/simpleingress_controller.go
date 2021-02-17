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
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/listers/core/v1"
	"k8s.io/klog"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	newgroupv1 "simple-ingress/api/v1"
	"strings"
	"time"
	"k8s.io/client-go/informers"
	clientConfig "sigs.k8s.io/controller-runtime/pkg/client/config"

)

// SimpleIngressReconciler reconciles a SimpleIngress object
type SimpleIngressReconciler struct {
	client.Client
	Log          logr.Logger
	Scheme       *runtime.Scheme
	RoutingTable map[string]*url.URL
	serviceLister v1.ServiceLister
}

func NewSimpleIngressReconciler(client  client.Client ,scheme *runtime.Scheme) *SimpleIngressReconciler{
	k8sClientConfig := clientConfig.GetConfigOrDie()
	k8sClient, err :=kubernetes.NewForConfig(k8sClientConfig)
	if err != nil {
		klog.Error("Failed to create client: ", err)
	}
	factory := informers.NewSharedInformerFactory(k8sClient, time.Minute)

	return &SimpleIngressReconciler{
		Client:       client,
		Log:          ctrl.Log.WithName("controllers").WithName("SimpleIngress"),
		Scheme:      scheme,
		RoutingTable: make(map[string]*url.URL),
		serviceLister: factory.Core().V1().Services().Lister(),
	}

}

// +kubebuilder:rbac:groups=newgroup.adva.domain,resources=simpleingresses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=newgroup.adva.domain,resources=simpleingresses/status,verbs=get;update;patch

func (sir *SimpleIngressReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	_ = context.Background()
	_ = sir.Log.WithValues("simpleingress", req.NamespacedName)

	klog.Infof("SimpleIngress reconcile is handling resource: %s", req.Name)

	// Get simple ingress crd by request name
	simpleIngress := &newgroupv1.SimpleIngress{}
	err := sir.Client.Get(context.TODO(), req.NamespacedName, simpleIngress)
	if err != nil {
		klog.Errorf("%s/%s: Could not get SimpleIngress", req.Namespace, req.Name)
		return reconcile.Result{}, nil
	}
	// Handle SimpleIngress delete - remove specific simpleingress rules and routs
	if simpleIngress.ObjectMeta.DeletionTimestamp != nil {
		klog.Infof("simpleIngress.ObjectMeta.DeletionTimestamp: %s", simpleIngress.ObjectMeta.DeletionTimestamp)
		sir.deleteRouts(simpleIngress.Spec.Rules)
		return ctrl.Result{}, nil
	}


	// Add or update routs according to simpleIngress rules
	for _, rule := range simpleIngress.Spec.Rules {
		//TODO: for each rule check that the backend exist
		_, err := sir.serviceLister.Services("service").Get(rule.BackendService.BackendServiceName)
		if err != nil {
			klog.Errorf("Failed to get service: %s", rule.BackendService.BackendServiceName)
		} else {
			// if exist add it to the routing table
			sir.createOrUpdateBackendRout(rule.Host, rule.BackendService.BackendServiceName, rule.BackendService.BackendServicePort)
		}
	}

	return ctrl.Result{}, nil
}


func (sir *SimpleIngressReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&newgroupv1.SimpleIngress{}).
		Complete(sir)
}

func (sir *SimpleIngressReconciler) deleteRouts(rules []newgroupv1.IngressRule){
	for _, rule := range rules {
		if url, ok := sir.RoutingTable[rule.Host]; ok {
			delete(sir.RoutingTable, rule.Host)
			klog.Infof("deleted rout %s --> %v", rule.Host, url.Host)
		}
	}

}

func (sir *SimpleIngressReconciler) createOrUpdateBackendRout(host string, serviceName string, servicePort int) {
	state := "create"
	if _, ok := sir.RoutingTable[host]; ok {
		state = "update"
	}

	sir.RoutingTable[host] = &url.URL{
		Scheme: "http",
		Host:   fmt.Sprintf("%s:%d", serviceName, servicePort),
	}
	klog.Infof("%s rout %s --> %s:%d", state, host, serviceName, servicePort)
}

// GetBackend gets the backends map and host and returns the url for the given host.
func (s *SimpleIngressReconciler) GetBackendURL(host string) (*url.URL, error) {
	// strip the port
	if idx := strings.IndexByte(host, ':'); idx > 0 {
		host = host[:idx]
	}
	if backendURL, ok := s.RoutingTable[host]; ok {
		return backendURL, nil
	}
	return nil, errors.New("backend not found")
}

// ServeHTTP serves an HTTP request.
func (s *SimpleIngressReconciler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	backendURL, err := s.GetBackendURL(r.Host)
	if err != nil {
		http.Error(w, "upstream server not found", http.StatusNotFound)
		return
	}
	klog.Infof("host %s , path %s , beckend %s proxing request", r.Host, r.URL.Path, backendURL.String())
	proxy := httputil.NewSingleHostReverseProxy(backendURL)
	proxy.ServeHTTP(w, r)
}
