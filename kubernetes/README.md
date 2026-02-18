# Kubernetes (referencia)

Manifiestos base para desplegar ExpenseLog en un cluster simple.

## Aplicar recursos

```bash
kubectl apply -f kubernetes/namespace.yml
kubectl apply -f kubernetes/expenselog-configmap.yml
kubectl apply -f kubernetes/expenselog-pvc.yml
kubectl apply -f kubernetes/expenselog-deployment.yml
kubectl apply -f kubernetes/expenselog-svc.yml
kubectl apply -f kubernetes/expenselog-ingress.yml
```

## Notas

- Namespace: `expenselog`
- Host de ejemplo: `expenselog.localhost`
- Imagen por defecto: `ghcr.io/joaquingonzalezzanotti/expenselog:main`
- Ajusta `storageClassName`, dominio e imagen para tu entorno.

