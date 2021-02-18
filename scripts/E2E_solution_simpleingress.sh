#! /bin/sh

# Create kind cluster and log cluster info
echo "Creare kind cluster simple-ingress-cluster"
kind create cluster --name simpleingress-cluster
kubectl cluster-info --context kind-simpleingress-cluster
echo "simple-ingress-cluster was created"

# Load the simple ingress controller docker image which is on dockerHub
kind load docker-image advaephraim/simpleingress:v.0.0.2
echo "Docker image advaephraim/simpleingress:v.0.0.2 was loaded to cluster simpleingress-cluster"


# Deploy simple echo server and expose it with a service to a simple imgress


# make http call using curl to the echo server and validat ingress reconcile

