/*
Copyright 2024.

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

package controller

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/go-logr/logr"
	linodego "github.com/linode/linodego"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"

	"operator.linode.io/vlanoperator/internal/helper"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	networkv1alpha1 "operator.linode.io/vlanoperator/api/v1alpha1"
)

// VlanReconciler reconciles a Vlan object
type VlanReconciler struct {
	client.Client
	Log      logr.Logger
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=network.operator.linode.io,resources=vlans,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=network.operator.linode.io,resources=vlans/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=network.operator.linode.io,resources=vlans/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Vlan object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.18.2/pkg/reconcile
func (r *VlanReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	klog := log.FromContext(ctx)

	//klog.Info("Reconciliation Started")
	klog.Info("Reconciliation Started", "Request.Namespace", req.Namespace, "Request.Name", req.Name)

	vlan := &networkv1alpha1.Vlan{}
	err := r.Get(ctx, req.NamespacedName, vlan)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			klog.Error(err, "Vlan resource not found, likely deleted", "Request", req)
			return ctrl.Result{}, nil
		}
		klog.Error(err, "Error getting Vlan")
		return ctrl.Result{}, err
	}

	//get vlan cidr from vlan object
	vlanCidr := vlan.Spec.VlanCidr
	vlanLabel := vlan.Spec.VlanLabel
	//get api key from secret
	apiKey, err := r.getAPIKeyFromSecret(ctx, vlan.Spec.ApiKeySecret, vlan)
	if err != nil {
		klog.Error(err, "Failed to get API Key from secret")
		r.Recorder.Eventf(vlan, corev1.EventTypeWarning, "APIKeyError", "Failed to get API Key: %v", err)
		return ctrl.Result{}, err
	}

	linodeClient := linodego.NewClient(nil)
	linodeClient.SetToken(apiKey)

	//get current list of nodes from kubernetes
	nodeMap, _, err := r.getClusterNodes(ctx, klog)
	if err != nil {
		klog.Error(err, "Failed to get cluster nodes")
		return ctrl.Result{}, err
	}

	//get current list of vlan in the cluster via linode api
	vlanNodeMap, err := r.getNodesInVlan(ctx, vlanLabel, &linodeClient)
	if err != nil {
		klog.Error(err, "Failed to get list of nodes in VLAN with label", "VLANLabel", vlanLabel)
		return ctrl.Result{}, err
	}

	//get a list of already assigned vlan ip addresses from linode
	assignedVLANIPMap, err := r.getMapOfAssignedIPs(ctx, vlanNodeMap, &linodeClient, vlanLabel)
	if err != nil {
		klog.Error(err, "Failed to get map of assigned IPs")
		return ctrl.Result{}, err
	}

	//print assignedIPMap as a single Info log
	klog.Info("Get nodes assigned within VLAN", "VLANLabel", vlanLabel, "AssignedIPMap", assignedVLANIPMap)

	//initialise IPAM by providing the range and also adding the already assigned ip addresses
	ipam, err := helper.NewIPAM(vlan.Spec.VlanCidr)
	if err != nil {
		klog.Error(err, "Failed to initialise IPAM")
		return ctrl.Result{}, err
	}

	for _, vlanip := range assignedVLANIPMap {
		//convert vlanip that looks like this 10.0.0.4/20 to 10.0.0.4
		ip := strings.Split(vlanip, "/")[0]
		err := ipam.MarkIPAsAssigned(ip)
		if err != nil {
			//if its warning, just log it and continue
			switch e := err.(type) {
			case *helper.IPAMWarning:
				r.Recorder.Eventf(vlan, corev1.EventTypeWarning, "IPAMWarning", "Warning: %s", e.Message)
			case *helper.IPAMError:
				r.Recorder.Eventf(vlan, corev1.EventTypeWarning, "IPAMError", "Error: %s", e.Message)
				return ctrl.Result{}, err
			default:
				r.Recorder.Eventf(vlan, corev1.EventTypeWarning, "IPAMError", "Error: %s", err)
				return ctrl.Result{}, err
			}
		}
	}

	addVLANList, err := r.CompareNodeandVLANMap(nodeMap, vlanNodeMap)
	if err != nil {
		klog.Error(err, "Failed to compare node and VLAN map")
		return ctrl.Result{}, err
	}

	err = r.addVLANToNode(ctx, vlan, addVLANList, ipam, vlanLabel, vlanCidr, &linodeClient, klog)
	if err != nil {
		klog.Error(err, "addNodesToVLAN failed")
		return ctrl.Result{}, err
	}

	//Identify duplicate IPs for reassignment
	duplicateVLANIPList, err := r.getDuplicateVLANIPs(assignedVLANIPMap)
	if err != nil {
		klog.Error(err, "Failed to get duplicate VLAN IPs")
		return ctrl.Result{}, err
	}

	err = r.updateVLANToNode(ctx, vlan, duplicateVLANIPList, ipam, vlanLabel, vlanCidr, &linodeClient, klog)
	if err != nil {
		klog.Error(err, "Failed to update VLAN to node")
		return ctrl.Result{}, err
	}

	klog.Info("Reconciliation completed successfully", "VLANLabel", vlanLabel)

	// return ctrl.Result{}, nil

	if vlan.Spec.Interval.Duration > 0 {
		//requeue after interval
		return ctrl.Result{RequeueAfter: vlan.Spec.Interval.Duration}, nil
	}
	// Default to requeue after 1min
	return ctrl.Result{RequeueAfter: 60 * time.Second}, nil
}

func (r *VlanReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&networkv1alpha1.Vlan{}, builder.WithPredicates(r.ignoreStatusUpdatesPredicate())).
		Watches(
			&corev1.Node{},
			handler.EnqueueRequestsFromMapFunc(r.nodeToVlan),
			builder.WithPredicates(r.ignoreNodeUpdatesWhenProviderIDNotPopulated()),
		).
		Complete(r)
}

// Only allow events that changes the specs
func (r *VlanReconciler) ignoreStatusUpdatesPredicate() predicate.Predicate {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldObj := e.ObjectOld.(*networkv1alpha1.Vlan)
			newObj := e.ObjectNew.(*networkv1alpha1.Vlan)
			// Ignore status updates
			return !reflect.DeepEqual(oldObj.Spec, newObj.Spec)
		},
	}
}

func (r *VlanReconciler) ignoreNodeUpdatesWhenProviderIDNotPopulated() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			// If providerID has not been populated, dont trigger reconcile
			// Rely on update instead
			return e.Object.GetAnnotations()["linode.com/providerID"] != ""
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			// Can ignore delete event as vlan will automatically be released
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldNode, oldOK := e.ObjectOld.(*corev1.Node)
			newNode, newOK := e.ObjectNew.(*corev1.Node)
			if !oldOK || !newOK {
				return false
			}
			// Check if providerID has been updated
			return oldNode.Spec.ProviderID != newNode.Spec.ProviderID
		},
		GenericFunc: func(e event.GenericEvent) bool {
			// Ignore generic events
			return false
		},
	}
}

// nodeToVlan is a handler function that maps Node events to vlan reconcile requests
func (r *VlanReconciler) nodeToVlan(ctx context.Context, obj client.Object) []reconcile.Request {
	// Assuming you have only one vlan resource
	vlans := &networkv1alpha1.VlanList{}
	err := r.List(ctx, vlans)
	if err != nil {
		return nil
	}

	var requests []reconcile.Request
	for _, vlan := range vlans.Items {
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      vlan.Name,
				Namespace: vlan.Namespace,
			},
		})
	}
	return requests
}

func (r *VlanReconciler) getAPIKeyFromSecret(ctx context.Context, secretRef networkv1alpha1.SecretReference, vlan *networkv1alpha1.Vlan) (string, error) {
	secret := &corev1.Secret{}
	if secretRef.Namespace == "" {
		secretRef.Namespace = vlan.Namespace
	}
	err := r.Get(ctx, types.NamespacedName{Name: secretRef.Name, Namespace: secretRef.Namespace}, secret)
	if err != nil {
		return "", fmt.Errorf("failed to get secret: %w", err)
	}

	apiKey, ok := secret.Data[secretRef.Key]
	if !ok {
		return "", fmt.Errorf("API key not found in secret under key: %s", secretRef.Key)
	}

	return string(apiKey), nil
}

// function to get list of nodes in the vlan via linode client
func (r *VlanReconciler) getNodesInVlan(ctx context.Context, vlanLabel string, linodeClient *linodego.Client) (map[int]int, error) {

	vlanNodeMap := make(map[int]int)
	f := linodego.Filter{}
	f.AddField(linodego.Eq, "label", vlanLabel)
	fStr, err := f.MarshalJSON()
	if err != nil {
		return nil, fmt.Errorf("failed to form filter: %w", err)
	}
	opts := linodego.NewListOptions(0, string(fStr))
	//filter by vlanLabel
	vlans, err := linodeClient.ListVLANs(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to list instances: %w", err)
	}
	for _, vlan := range vlans {
		if vlan.Label == vlanLabel {
			for _, node := range vlan.Linodes {
				//add to a map
				vlanNodeMap[node] = node
			}
		}
	}
	return vlanNodeMap, nil
}

func (r *VlanReconciler) getClusterNodes(ctx context.Context, klog logr.Logger) (map[int]string, []string, error) {
	nodeMap := make(map[int]string)
	nodeList := []string{}
	nodes := &corev1.NodeList{}

	if err := r.Client.List(ctx, nodes); err != nil {
		return nil, nil, fmt.Errorf("unable to get list of nodes from Kubernetes: %w", err)
	}

	for _, node := range nodes.Items {
		nodeID, err := strconv.Atoi(strings.TrimPrefix(node.Spec.ProviderID, "linode://"))
		if err != nil {
			klog.Error(err, "Failed to convert node ID to integer", "ProviderID", node.Spec.ProviderID, "NodeName", node.Name)
			continue
		}
		if nodeID == 0 {
			klog.Info("Recieved ProviderID as 0, skipping", "NodeID", nodeID)
			continue
		}
		nodeMap[nodeID] = node.Name
		nodeList = append(nodeList, node.Name)
	}

	return nodeMap, nodeList, nil
}

func (r *VlanReconciler) CompareNodeandVLANMap(nodeMap map[int]string, vlanNodeMap map[int]int) ([]int, error) {
	//Only add list is required.
	addList := []int{}
	// print vlannodemap
	for k := range nodeMap {
		//if key nodeMap is not present in key of vlanNodeMap, then add to a addList array
		if _, ok := vlanNodeMap[k]; !ok {
			addList = append(addList, k)
		}
	}
	return addList, nil
}

// func (r *VlanReconciler) assignVlanToNodes(ctx context.Context, vlanLabel string, linodeClient *linodego.Client, addList []int) error {

// 	//test
// }

func (r *VlanReconciler) getMapOfAssignedIPs(ctx context.Context, vlanNodeMap map[int]int, linodeClient *linodego.Client, vlanLabel string) (map[int]string, error) {
	assignedIPMap := make(map[int]string)
	//get the current list of vlan ips from linode go client
	for id := range vlanNodeMap {
		configs, err := linodeClient.ListInstanceConfigs(ctx, id, nil)
		//assuming LKE always use Boot Config at index 0
		for _, configinterface := range configs[0].Interfaces {
			if configinterface.Label == vlanLabel {
				assignedIPMap[id] = configinterface.IPAMAddress
			}
		}
		if err != nil {
			return nil, fmt.Errorf("failed to get instance config %d: %w", id, err)
		}
	}
	return assignedIPMap, nil
}

// selects a duplicated vlan node id for vlan reassignment
func (r *VlanReconciler) getDuplicateVLANIPs(assignedVLANIPMap map[int]string) ([]int, error) {
	ipToNodeMap := make(map[string][]int) // Map to track which nodes have which IPs
	duplicateVLANIPList := []int{}

	for nodeID, vlanIP := range assignedVLANIPMap {
		ipToNodeMap[vlanIP] = append(ipToNodeMap[vlanIP], nodeID)
	}

	for _, nodeIDs := range ipToNodeMap {
		if len(nodeIDs) > 1 {
			// Skip the first node ID and add the rest to the duplicate list
			duplicateVLANIPList = append(duplicateVLANIPList, nodeIDs[1:]...)
		}
	}

	return duplicateVLANIPList, nil
}

func (r *VlanReconciler) addVLANToNode(ctx context.Context, vlan *networkv1alpha1.Vlan, addVLANList []int, ipam *helper.IPAM, vlanLabel string, vlanCidr string, linodeClient *linodego.Client, klog logr.Logger) error {

	for _, linodeId := range addVLANList {
		//assign a vlanIP
		vlanIP, err := ipam.AssignIP()
		if err != nil {
			return fmt.Errorf("failed to assign IP: %w", err)
		}
		//append the /xx from vlancidr
		vlanIP = vlanIP + "/" + vlanCidr[strings.Index(vlanCidr, "/")+1:]

		klog.Info("Assigning VLAN to node", "NodeID", linodeId, "VLANIP", vlanIP)

		//create vlanInterface
		vlanInterface := linodego.InstanceConfigInterfaceCreateOptions{
			IPAMAddress: vlanIP,
			Label:       vlanLabel,
			Purpose:     "vlan",
			Primary:     false,
		}
		//get instance config
		configs, err := linodeClient.ListInstanceConfigs(ctx, linodeId, nil)
		if err != nil {
			return fmt.Errorf("failed to get instance config %d: %w", linodeId, err)
		}
		configId := configs[0].ID

		//It is possible that the instance already has a vlan interface,
		//so we need to check if it already exists
		// Vlan list is not an atomic operation
		for _, configInterface := range configs[0].Interfaces {
			if configInterface.Purpose == "vlan" && configInterface.Label == vlanLabel {
				klog.Info("VLAN already exists on node", "NodeID", linodeId, "VLANLabel", vlanLabel, "existingvlanIP", configInterface.IPAMAddress)
				return nil
			}
		}

		//if current interfaces is empty, ensure that we assign the first interface with public ip
		if len(configs[0].Interfaces) == 0 {
			internetInterface := linodego.InstanceConfigInterfaceCreateOptions{
				IPAMAddress: "",
				Label:       "",
				Purpose:     "public",
				Primary:     false,
			}
			_, err := linodeClient.AppendInstanceConfigInterface(ctx, linodeId, configId, internetInterface)
			if err != nil {
				return fmt.Errorf("failed to create internet interface: %w", err)
			}
		}
		_, err = linodeClient.AppendInstanceConfigInterface(ctx, linodeId, configId, vlanInterface)
		if err != nil {
			return fmt.Errorf("failed to create vlan interface: %w", err)
		}

		//reboot the server
		err = linodeClient.RebootInstance(ctx, linodeId, configId)
		if err != nil {
			return fmt.Errorf("failed to reboot instance: %w", err)
		}

		r.Recorder.Eventf(vlan, corev1.EventTypeNormal, "VLANAssigned", "VLAN assigned to node %d and reboot triggered", linodeId)

	}
	return nil
}

// Update VLAN to node
func (r *VlanReconciler) updateVLANToNode(ctx context.Context, vlan *networkv1alpha1.Vlan, duplicateVLANIPList []int, ipam *helper.IPAM, vlanLabel string, vlanCidr string, linodeClient *linodego.Client, klog logr.Logger) error {
	for _, linodeId := range duplicateVLANIPList {
		//assign a vlanIP
		vlanIP, err := ipam.AssignIP()
		if err != nil {
			return fmt.Errorf("failed to assign IP: %w", err)
		}
		//append the /xx from vlancidr
		vlanIP = vlanIP + "/" + vlanCidr[strings.Index(vlanCidr, "/")+1:]

		klog.Info("Assigning VLAN to node", "NodeID", linodeId, "VLANIP", vlanIP)

		//create vlanInterface
		// vlanInterface := linodego.InstanceConfigInterfaceCreateOptions{
		// 	IPAMAddress: vlanIP,
		// 	Label:       vlanLabel,
		// 	Purpose:     "vlan",
		// 	Primary:     false,
		// }

		//get instance config
		configs, err := linodeClient.ListInstanceConfigs(ctx, linodeId, nil)
		if err != nil {
			return fmt.Errorf("failed to get instance config %d: %w", linodeId, err)
		}
		configId := configs[0].ID

		instanceConfigUpdateOptions := linodego.InstanceConfigUpdateOptions{
			Interfaces: []linodego.InstanceConfigInterfaceCreateOptions{
				{
					IPAMAddress: "",
					Label:       "",
					Purpose:     "public",
					Primary:     false,
				},
				{
					IPAMAddress: vlanIP,
					Label:       vlanLabel,
					Purpose:     "vlan",
					Primary:     false,
				},
			},
		}

		//update instance config
		_, err = linodeClient.UpdateInstanceConfig(ctx, linodeId, configId, instanceConfigUpdateOptions)
		if err != nil {
			return fmt.Errorf("failed to update instance config: %w", err)
		}

		err = linodeClient.RebootInstance(ctx, linodeId, configId)
		if err != nil {
			return fmt.Errorf("failed to reboot instance: %w", err)
		}

		r.Recorder.Eventf(vlan, corev1.EventTypeNormal, "VLANAssigned", "VLAN assigned to node %d and reboot triggered", linodeId)

	}

	return nil

}
