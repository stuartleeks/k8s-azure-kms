# k8s-azure-kms #

Azure KMS plugin for Kubernetes - Enable encryption of eecret data at rest in Kubernetes using Azure Key Vault

**Project Status**: Alpha

## How to use ##

### Prerequisites: ### 

Make sure you have a Kubernetes cluster v1.10+, as you will need the [PR](https://github.com/kubernetes/kubernetes/pull/55684) that added the gRPC-based KMS plugin service. You can also use my image [ritazh/hyperkube-amd64:v1.10.3](https://hub.docker.com/r/ritazh/hyperkube-amd64) if you don't want to build your own.

> :triangular_flag_on_post: NOTE: Until the end to end has been added to acs-engine, you will need to do the following manually.

### Configurations ###

From all master nodes:

1. Create `/etc/kubernetes/manifests/encryptionconfig.yaml`

```yaml
kind: EncryptionConfig
apiVersion: v1
resources:
  - resources:
    - secrets
    providers:
    - kms:
        name: azurekmsprovider
        endpoint: unix:///tmp/azurekms.socket
        cachesize: 0
    - identity: {}
```

2. Modify `/etc/kubernetes/manifests/kube-apiserver.yaml` 
Add the following flag:

```yaml
--experimental-encryption-provider-config=/etc/kubernetes/manifests/encryptionconfig.yaml
```  
Mount `/tmp` to access the socket:

```yaml
...
 volumeMounts:
        - name: "sock"
          mountPath: "/tmp"
...
 volumes:
    - name: "sock"
      hostPath:
        path: "/tmp"

```

3. Update `/etc/kubernetes/azure.json` to add the following configurations:

```json
...
    "providerVaultBaseURL": "",
    "providerKeyName": "",
    "providerKeyVersion": ""

```
* `providerVaultBaseURL`: should look something like this: `https://yourkeyvaultname.vault.azure.net/`
* `providerKeyName`: name of the key created in azure key vault
* `providerKeyVersion`: key version of the key created in azure key vault

4. Drop [`kube-azurekmspod.yaml`](kubernetes/kube-azurekmspod.yaml) under `/etc/kubernetes/manifests`, kubelet will create a static pod that starts the gRPC service. The pod will be named similar to `azurekms-k8s-master-32960228-0`. To verify the gRPC service is running,  you should see the following from the pod logs. You should also see the /tmp/azurekms.socket created.

```bash
$ kubectl logs azurekms-k8s-master-32960228-0 
/tmp/azurekms.socket
2018/02/26 22:52:26 KMSServiceServer service starting...
2018/02/26 22:52:26 KMSServiceServer service started successfully.

ls /tmp/azu*
/tmp/azurekms.socket
```

5. Restart apiserver

## Verifying that Data is Encrypted ##

Now that your cluster has `--experimental-encryption-provider-config` turned on, it will use the information provided to encrypt the data in etcd. 

1. Create a new secret

```bash
kubectl create secret generic secret1 -n default --from-literal=mykey=mydata
```

2. Using etcdctl, read the secret out of the etcd:

```bash
sudo ETCDCTL_API=3 etcdctl --cacert=/etc/kubernetes/certs/ca.crt --cert=/etc/kubernetes/certs/etcdclient.crt --key=/etc/kubernetes/certs/etcdclient.key get /registry/secrets/default/secret1
```

3. Verify the stored secret is prefixed with `k8s:enc:kms:v1:azurekmsprovider` which indicates the azure kms provider has encrypted the resulting data.

4. Verify the secret is correctly decrypted when retrieved via the API:

```bash
kubectl get secrets secret1 -o yaml
```
the output should match `mykey: bXlkYXRh`, which is the encoded data of `mydata`. 



