// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package iac

import "fmt"

// KubernetesRenderer produces Kubernetes manifests (Deployment, Service, NetworkPolicy).
type KubernetesRenderer struct{}

func (r *KubernetesRenderer) Backend() Backend { return BackendKubernetes }

func (r *KubernetesRenderer) Render(config ServiceConfig) (*RenderOutput, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	out := &RenderOutput{
		Backend: BackendKubernetes,
		Files:   make(map[string]string),
	}

	out.Files[fmt.Sprintf("%s/deployment.yaml", config.Name)] = r.renderDeployment(config)
	out.Files[fmt.Sprintf("%s/service.yaml", config.Name)] = r.renderService(config)
	out.Files[fmt.Sprintf("%s/networkpolicy.yaml", config.Name)] = r.renderNetworkPolicy(config)

	return out, nil
}

func (r *KubernetesRenderer) RenderAll(configs []ServiceConfig) (*RenderOutput, error) {
	out := &RenderOutput{
		Backend: BackendKubernetes,
		Files:   make(map[string]string),
	}

	for _, cfg := range configs {
		single, err := r.Render(cfg)
		if err != nil {
			return nil, fmt.Errorf("render %s: %w", cfg.Name, err)
		}
		for k, v := range single.Files {
			out.Files[k] = v
		}
	}

	out.Files["namespace.yaml"] = r.renderNamespace()
	out.Files["kustomization.yaml"] = r.renderKustomization(configs)

	return out, nil
}

func (r *KubernetesRenderer) renderDeployment(config ServiceConfig) string {
	image := config.Image
	if image == "" {
		image = fmt.Sprintf("unheaded/%s:latest", config.Name)
	}

	s := fmt.Sprintf(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: %s
  namespace: unheaded
  labels:
    app.kubernetes.io/name: %s
    app.kubernetes.io/part-of: unheaded
spec:
  replicas: %d
  selector:
    matchLabels:
      app.kubernetes.io/name: %s
  template:
    metadata:
      labels:
        app.kubernetes.io/name: %s
    spec:
      securityContext:
        runAsNonRoot: true
`, config.Name, config.Name, config.Replicas, config.Name, config.Name)

	if config.Hardening.RunAsUser > 0 {
		s += fmt.Sprintf("        runAsUser: %d\n        runAsGroup: %d\n",
			config.Hardening.RunAsUser, config.Hardening.RunAsGroup)
	}

	s += fmt.Sprintf(`      containers:
        - name: %s
          image: %s
          ports:
            - containerPort: %d
              name: http
              protocol: TCP
`, config.Name, image, config.Port)

	if config.GRPCPort > 0 {
		s += fmt.Sprintf(`            - containerPort: %d
              name: grpc
              protocol: TCP
`, config.GRPCPort)
	}

	// Environment
	if len(config.Environment) > 0 {
		s += "          env:\n"
		for k, v := range config.Environment {
			s += fmt.Sprintf("            - name: %s\n              value: \"%s\"\n", k, v)
		}
	}

	// Resources
	s += fmt.Sprintf(`          resources:
            limits:
              cpu: "%s"
              memory: "%s"
            requests:
              cpu: "%s"
              memory: "%s"
`, or(config.Resources.CPULimit, "500m"),
		or(config.Resources.MemLimit, "256Mi"),
		or(config.Resources.CPURequest, "100m"),
		or(config.Resources.MemRequest, "64Mi"))

	// Health probes
	s += fmt.Sprintf(`          livenessProbe:
            httpGet:
              path: %s
              port: http
            initialDelaySeconds: 5
            periodSeconds: 10
          readinessProbe:
            httpGet:
              path: %s
              port: http
            initialDelaySeconds: 3
            periodSeconds: 5
`, config.HealthPath, config.ReadyPath)

	// Security context
	s += "          securityContext:\n"
	if config.Hardening.ReadOnlyFS {
		s += "            readOnlyRootFilesystem: true\n"
	}
	if config.Hardening.NoNewPrivs {
		s += "            allowPrivilegeEscalation: false\n"
	}
	if config.Hardening.DropAllCaps {
		s += "            capabilities:\n              drop:\n                - ALL\n"
		if len(config.Hardening.AllowedCaps) > 0 {
			s += "              add:\n"
			for _, cap := range config.Hardening.AllowedCaps {
				s += fmt.Sprintf("                - %s\n", cap)
			}
		}
	}
	if config.Hardening.SeccompPolicy != "" {
		s += fmt.Sprintf("            seccompProfile:\n              type: RuntimeDefault\n")
	}

	return s
}

func (r *KubernetesRenderer) renderService(config ServiceConfig) string {
	s := fmt.Sprintf(`apiVersion: v1
kind: Service
metadata:
  name: %s
  namespace: unheaded
  labels:
    app.kubernetes.io/name: %s
spec:
  selector:
    app.kubernetes.io/name: %s
  ports:
    - name: http
      port: %d
      targetPort: http
      protocol: TCP
`, config.Name, config.Name, config.Name, config.Port)

	if config.GRPCPort > 0 {
		s += fmt.Sprintf(`    - name: grpc
      port: %d
      targetPort: grpc
      protocol: TCP
`, config.GRPCPort)
	}

	return s
}

func (r *KubernetesRenderer) renderNetworkPolicy(config ServiceConfig) string {
	s := fmt.Sprintf(`apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: %s
  namespace: unheaded
spec:
  podSelector:
    matchLabels:
      app.kubernetes.io/name: %s
  policyTypes:
    - Ingress
    - Egress
  ingress:
    - from:
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: unheaded
      ports:
        - port: %d
          protocol: TCP
`, config.Name, config.Name, config.Port)

	if config.GRPCPort > 0 {
		s += fmt.Sprintf(`        - port: %d
          protocol: TCP
`, config.GRPCPort)
	}

	s += `  egress:
    - to:
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: unheaded
    - ports:
        - port: 53
          protocol: UDP
        - port: 53
          protocol: TCP
`
	return s
}

func (r *KubernetesRenderer) renderNamespace() string {
	return `apiVersion: v1
kind: Namespace
metadata:
  name: unheaded
  labels:
    app.kubernetes.io/part-of: unheaded
`
}

func (r *KubernetesRenderer) renderKustomization(configs []ServiceConfig) string {
	s := "apiVersion: kustomize.config.k8s.io/v1beta1\nkind: Kustomization\nnamespace: unheaded\nresources:\n  - namespace.yaml\n"
	for _, cfg := range configs {
		s += fmt.Sprintf("  - %s/deployment.yaml\n  - %s/service.yaml\n  - %s/networkpolicy.yaml\n",
			cfg.Name, cfg.Name, cfg.Name)
	}
	return s
}

func or(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
