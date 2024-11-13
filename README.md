# OpenShift Shared Resource CSI Driver

The OpenShift Shared Resource CSI Driver allows `Secrets` and `ConfigMaps` to be
shared across Kubernetes namespaces in a controlled manner. This CSI driver
ensures that the entity (ServiceAccount) accessing the shared Secret or ConfigMap
has permission to do so before mounting the data as a volume into the requesting
Pod.

This CSI driver only supports the `Ephemeral` volume lifecycle mode.
It also requires the following during operation:

- `podInfoOnMount: true`
- `fsGroupPolicy: File`
- `attachRequired: false`

## Getting Started

The easiest way to use the Shared Resource CSI Driver is to deploy OpenShift
v4.10 or higher, and enable the
[Tech Preview Features](https://docs.openshift.com/container-platform/latest/post_installation_configuration/cluster-tasks.html#post-install-tp-tasks).

## How To Use

Typically there are two individuals/personas involved when sharing resources:

- A "resource owner" - a platform engineer or other person granted the 
  [admin role](https://kubernetes.io/docs/reference/access-authn-authz/rbac/#user-facing-roles)
  in multiple application namespaces. This could also be a cluster administrator.
- A "resource consumer" - an application developer who is granted the
  [edit role](https://kubernetes.io/docs/reference/access-authn-authz/rbac/#user-facing-roles)
  in a namespace.

Sharing resources is done as follows:

1. The resource owner creates a `Secret` or `ConfigMap` to be shared in a
   "source" namespace. This could also be created by a controller or other system
   component.

   ```yaml
   apiVersion: v1
   kind: ConfigMap
   metadata:
     name: shared-config
     namespace: default # This can be any desired "source" namespace
   data:
     config.txt: "Hello world!"
   ```

2. The resource owner should create a corresponding `SharedSecret` or `SharedConfigMap` instance to make the resource shareable:

   ```yaml
   apiVersion: sharedresource.openshift.io/v1alpha1
   kind: SharedConfigMap
   metadata:
     name: share-default-config
   spec:
     configMapRef:
       name: shared-config
       namespace: default
   ```

3. The resource owner then creates a `Clusterrole` and `Clusterrolebinding` to grant permission 
to the `ServiceAccount` of `csi-driver-shared-resource` to access 
the given resources.

   ```yaml
   ---
   apiVersion: rbac.authorization.k8s.io/v1
   kind: ClusterRole
   metadata:
     name: shared-resource-secret-configmap-share-watch-sar-create
   rules:
     - apiGroups: [""]
       resources: ["configmaps"]
       resourceNames: ["shared-config"]
       verbs: ["get", "list", "watch"]
     - apiGroups: ["sharedresource.openshift.io"]
       resources: ["sharedconfigmaps", "sharedsecrets"]
       verbs: ["get", "list", "watch"]
     - apiGroups: ["authorization.k8s.io"]
       resources: ["subjectaccessreviews"]
       verbs: ["create"]
   ---
    apiVersion: rbac.authorization.k8s.io/v1
    kind: ClusterRoleBinding
    metadata:
      name: shared-resource-secret-configmap-share-watch-sar-create
    roleRef:
      apiGroup: rbac.authorization.k8s.io
      kind: ClusterRole
      name: shared-resource-secret-configmap-share-watch-sar-create
    subjects:
    - kind: ServiceAccount
      name: csi-driver-shared-resource
      namespace: openshift-builds
    ```

4. The resource owner grants the desired `SeviceAccount` in the "target"
   namespace permission to use the shared resource above:

   ```yaml
   ---
   apiVersion: rbac.authorization.k8s.io/v1
   kind: Role
   metadata:
     name: use-shared-default-config
     namespace: app-namespace # This is the "target" namespace
   rules:
     - apiGroups:
         - sharedresource.openshift.io
       resources:
         - sharedconfigmaps
       resourceNames:
         - share-default-config
       verbs:
         - use
   ---
   apiVersion: rbac.authorization.k8s.io/v1
   kind: RoleBinding
   metadata:
     name: use-shared-default-config
     namespace: app-namespace
   roleRef:
     apiGroup: rbac.authorization.k8s.io
     kind: Role
     name: use-shared-default-config
   subjects:
     - kind: ServiceAccount
       name: default
       namespace: app-namespace
   ```

5. The resource consumer mounts the shared resource into a `Pod` (or other
   resource that accepts `CSI` Volumes):

   ```yaml
   apiVersion: v1
   kind: Pod
   metadata:
     name: example-shared-config
     namespace: app-namespace
   spec:
     ...
     serviceAccountName: default
     volumes:
       - name: shared-config
         csi:
           readOnly: true # required to be true
           driver: csi.sharedresource.openshift.io
           volumeAttributes:
             sharedConfigMap: share-default-config
   ```

See also:

- [Simple example](docs/simple-example.md)
- [Tekton example](docs/tekton-example.md)
- [OpenShift BuildConfig example](docs/build-with-rhel-entitlements.md)

## Features

- ServiceAccounts must have the `use` permission to mount the respective
  `SharedSecret` or `SharedConfigMap`. Volumes fail to mount otherwise - see 
  [FAQ](docs/faq.md) for more details.
- Automatic sync of the shared resource data (Secret/ConfigMap) into the mounting
  `Pod`.
- Automatic removal/restoration of shared resource data if the Pod's RBAC
  permissions change at runtime.
- Automatic removal/restoration of shared resource data if the backing
  Secret/ConfigMap is deleted/re-created.
- Survival of shared resource data with CSI driver restarts/upgrades.
- Multiple `SharedSecret`/`SharedConfig` volumes within a `Pod`. Also supports
  nested volume mounts within a container.
- Reserve a cluster-scoped share name to a specific `Secret` or `ConfigMap`.

The following CSI interfaces are implemented:

- **Identity Service**: GetPluginInfo, GetPluginCapabilities, Probe
- **Node Service**: NodeGetInfo, NodeGetCapabilities, NodePublishVolume,
  NodeUnpublishVolume
- **Controller Service**: _not implemented_.

NOTE: see [CSI Volume Specifics](docs/csi.md) for restrictions around these features for read-only Volumes.

## FAQ

Please refer to the [FAQ Guide](docs/faq.md) for commonly asked questions.

## Development

See the [development guide](docs/local-development.md) on how to build and test locally.
