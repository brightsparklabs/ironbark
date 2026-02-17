---
apiVersion: v1
kind: Secret
metadata:
  labels:
    argocd.argoproj.io/secret-type: repository
    zarf.dev/agent: ignore
  name: zarf-helm-oci
  namespace: ironbark-argocd
stringData:
  url: zarf-docker-registry.zarf.svc.cluster.local/ironbark-helm-charts
  username: {{.Username}}
  password: {{.Password}}
  type: helm
  enableOCI: "true"
  insecure: "true"
  insecureOCIForceHttp: "true"
