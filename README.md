# Deploy Linode VLAN Operator

Vlan Operator allows vlan IP address and custom routes to be assigned to every worker node in the cluster.
Two key custom resource definitions:
1. Vlan - Ensure vlan ip addresses are assigned to worker nodes within the cluster based on the vlan cidr and vlan label specified.
2. CustomStaticRoutes - provisioned a daemonset that ensure specified array of custom routes are injected in the host ip route table.

## Table of Contents
- [Prerequisites](#prerequisites)
- [Usage](#usage)
- [FAQ](#faq)
- [Troubleshooting Guide](#troubleshooting-guide)
- [Remarks](#remarks)

## Prerequisites

1. Linode Kubernetes Cluster with outbound connectivity to https://api.linode.com
2. Linode PAT Token with "Linode Read/Write" permissions

## Quick Start

1. **Deploy required CRD and Operator:**

   ```bash
   curl -s https://raw.githubusercontent.com/gangyi89/linode-vlan-operator/main/deploy/deploy-linode-vlan-operator.sh | bash -s -- <MY_NAMESPACE>
   ```

   > **Note:** Specify the desired namespace for the operator. If not specified, the default namespace will be used.

2. **Copy vlan and custom route objects:**

   1. Copy the `vlan-customroute.yaml` file in deploy folder
   2. Populate the Linode API Key, along with vlan and customroute details
   3. Deploy the firewall object to the same namespace:

      ```bash
      kubectl apply -f vlan-customroute.yaml -n <MY_NAMESPACE>
      ```

## FAQ

#### What does this operator do?

This operator performs two key operation:
**Assign vlan ip address to worker node**
- Based on the specified VLAN CIDR (with starting ip address) and VLAN label, all worker nodes will be assigned a vlan ip address.
- A worker node vlan ip address is randomly selected from a pool of available ip addresses (vlan cidr - vlan ips currently in use).
- A newly assigned node will be restarted in order for vlan to be applied on the host nic.

**Assign custom routes to worker node**
- This feature is particularly useful when worker nodes need to reach resources via a specific next hop. The custom route definition allows you to specify both the destination CIDR range and the next hop IP address.
- The operator creates a DaemonSet that periodically adds and maintains the custom routes in each node's IP routing table. This ensures that all nodes, including newly added ones, consistently have the expected routes configured.
- By using a DaemonSet, the operator guarantees that routes persist across node reboots and system changes, maintaining the desired network topology for your cluster.

#### What are the parameters available in vlan and static host routes?

##### Vlan
| Parameter | Type | Mandatory | Description |
|----------|----------|----------|----------|
| vlanCidr | string | Y | Vlan cidr range with starting ip address |
| vlanLabel | string | Y | Vlan label |
| apiKeySecret.name | string | Y | Name of kubernetes Secret |
| apiKeySecret.key | string | Y | Key of kubernetes Secret |
| apiKeySecret.namespace | string | N | If namespace is not specifed, namespace of the operator will be used |
| interval | string | N | Periodically performs reconciliation on top of node events. Format is in s, m, h, d. Default is 60s |

##### StaticHostRoutes
| Parameter | Type | Mandatory | Description |
|----------|----------|----------|----------|
| routes | array | Y | Specify a list of routes to be added in host route table |
| cidr | string | Y | The destination CIDR |
| gateway | string | Y | The next hop IP address |
| interval | string | N | Periodically performs reconciliation on top of node events. Format is in s, m, h, d. Default is 60s |

#### Can I provision the operator as 2 replicas instead of the default single replica instance?

Yes, you can, but it's not necessary. In the event that the node containing the operator is removed, the operator will be rescheduled to another node and perform reconciliation at start-up.

#### How do I secure my Linode API Key?

The operator is designed to consume the API Key from a Secret object. Hence, you can apply a consistent Kubernetes secrets management strategy to secure the API Key.

#### Can it handle LKE cluster autoscale, upgrades, recycle pool, and delete pool?

Yes! To the operator, the above operations are nothing but create and delete node events. The operator will handle all of these scenarios.

#### What happens when a vlan ip address is manually removed from a worker node instance?
A new vlan ip address will be assigned to the node in the next reconcilation cycle. Either due to a node add/delete detected by the cluster or after the interval (default 60s).

## Remarks
This project is of the work of an individual and is not affiliated with Linode in any way.