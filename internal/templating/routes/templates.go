package routes

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/uselagoon/build-deploy-tool/internal/helpers"
	"github.com/uselagoon/build-deploy-tool/internal/lagoon"
	networkv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"sigs.k8s.io/yaml"
)

// GenerateIngressTemplate generates the lagoon template to apply.
func GenerateIngressTemplate(route lagoon.RouteV2, lValues lagoon.BuildValues,
	monitoringContact, monitoringStatusPageID string,
	monitoringEnabled bool) []byte {

	// generate the name for the ingress fromt he route domain
	// shorten it if required with a hash
	ingressName := route.Domain
	if len(ingressName) >= 53 {
		ingressName = fmt.Sprintf("%s-%s", strings.Split(ingressName, ".")[0], helpers.GetMD5HashWithNewLine(ingressName)[:5])
	}

	// if a specific ingressname has been provided, use it instead
	if route.IngressName != "" {
		ingressName = route.IngressName
	}

	// create the ingress object for templating
	ingress := &networkv1.Ingress{}
	ingress.TypeMeta = metav1.TypeMeta{
		Kind:       "Ingress",
		APIVersion: "networking.k8s.io/v1",
	}
	ingress.ObjectMeta.Name = ingressName
	ingress.ObjectMeta.Labels = map[string]string{
		"lagoon.sh/autogenerated":      "false",
		"helm.sh/chart":                fmt.Sprintf("%s-%s", "custom-ingress", "0.1.0"),
		"app.kubernetes.io/name":       "custom-ingress",
		"app.kubernetes.io/instance":   route.Domain,
		"app.kubernetes.io/managed-by": "Helm",
		"lagoon.sh/service":            route.Domain,
		"lagoon.sh/service-type":       "custom-ingress",
		"lagoon.sh/project":            lValues.Project,
		"lagoon.sh/environment":        lValues.Environment,
		"lagoon.sh/environmentType":    lValues.EnvironmentType,
		"lagoon.sh/buildType":          lValues.BuildType,
	}
	additionalLabels := map[string]string{}
	if route.Migrate != nil {
		additionalLabels["dioscuri.amazee.io/migrate"] = strconv.FormatBool(*route.Migrate)
	} else {
		additionalLabels["dioscuri.amazee.io/migrate"] = "false"
	}
	for key, value := range additionalLabels {
		ingress.ObjectMeta.Labels[key] = value
	}
	ingress.ObjectMeta.Annotations = map[string]string{
		"kubernetes.io/tls-acme": strconv.FormatBool(*route.TLSAcme),
		"fastly.amazee.io/watch": strconv.FormatBool(route.Fastly.Watch),
		"lagoon.sh/version":      lValues.LagoonVersion,
	}
	additionalAnnotations := map[string]string{}
	additionalAnnotations["monitor.stakater.com/enabled"] = "false"
	if monitoringEnabled {
		// only add the monitring annotations if monitoring is enabled
		additionalAnnotations["monitor.stakater.com/enabled"] = "true"
		additionalAnnotations["uptimerobot.monitor.stakater.com/alert-contacts"] = "unconfigured"
		if monitoringContact != "" {
			additionalAnnotations["uptimerobot.monitor.stakater.com/alert-contacts"] = monitoringContact
		}
		if monitoringStatusPageID != "" {
			additionalAnnotations["uptimerobot.monitor.stakater.com/status-pages"] = monitoringStatusPageID
		}
		additionalAnnotations["uptimerobot.monitor.stakater.com/interval"] = "60"
	}
	if route.MonitoringPath != "" {
		additionalAnnotations["monitor.stakater.com/overridePath"] = route.MonitoringPath
	}
	if route.Fastly.ServiceID != "" {
		additionalAnnotations["fastly.amazee.io/service-id"] = route.Fastly.ServiceID
	}
	if route.Fastly.APISecretName != "" {
		additionalAnnotations["fastly.amazee.io/api-secret-name"] = route.Fastly.APISecretName
	}
	if lValues.BuildType == "branch" {
		additionalAnnotations["lagoon.sh/branch"] = lValues.Branch
	} else if lValues.BuildType == "pullrequest" {
		additionalAnnotations["lagoon.sh/prNumber"] = lValues.PRNumber
		additionalAnnotations["lagoon.sh/prHeadBranch"] = lValues.PRHeadBranch
		additionalAnnotations["lagoon.sh/prBaseBranch"] = lValues.PRBaseBranch

	}
	if *route.Insecure == "Allow" {
		additionalAnnotations["nginx.ingress.kubernetes.io/ssl-redirect"] = "false"
		additionalAnnotations["ingress.kubernetes.io/ssl-redirect"] = "false"
	} else if *route.Insecure == "Redirect" || *route.Insecure == "None" {
		additionalAnnotations["nginx.ingress.kubernetes.io/ssl-redirect"] = "true"
		additionalAnnotations["ingress.kubernetes.io/ssl-redirect"] = "true"
	}
	if lValues.EnvironmentType == "development" {
		additionalAnnotations["nginx.ingress.kubernetes.io/server-snippet"] = "add_header X-Robots-Tag \"noindex, nofollow\";\n"
	}
	for key, value := range additionalAnnotations {
		ingress.ObjectMeta.Annotations[key] = value
	}
	for key, value := range route.Annotations {
		ingress.ObjectMeta.Annotations[key] = value
	}
	for key, value := range route.Labels {
		ingress.ObjectMeta.Labels[key] = value
	}
	ingress.Spec.TLS = []networkv1.IngressTLS{
		{
			SecretName: fmt.Sprintf("%s-tls", ingressName),
		},
	}
	// autogenerated domains that are too long break when creating the acme challenge k8s resource
	// this injects a shorter domain into the tls spec that is used in the k8s challenge
	if lValues.ShortAutogeneratedRouteDomain != "" && len(route.Domain) > 63 {
		ingress.Spec.TLS[0].Hosts = append(ingress.Spec.TLS[0].Hosts, lValues.ShortAutogeneratedRouteDomain)
	}
	// add the main domain to the tls spec now
	ingress.Spec.TLS[0].Hosts = append(ingress.Spec.TLS[0].Hosts, route.Domain)

	pt := networkv1.PathTypePrefix

	// default service port is http
	servicePort := networkv1.ServiceBackendPort{
		Name: "http",
	}
	// if a port number is provided, use it
	if route.ServicePortNumber != nil {
		servicePort = networkv1.ServiceBackendPort{
			Number: *route.ServicePortNumber,
		}
	}
	// if a different port name is provided use it above all else
	if route.ServicePortName != nil {
		servicePort = networkv1.ServiceBackendPort{
			Name: *route.ServicePortName,
		}
	}

	// add the main domain as the first rule in the spec
	ingress.Spec.Rules = []networkv1.IngressRule{
		{
			Host: route.Domain,
			IngressRuleValue: networkv1.IngressRuleValue{
				HTTP: &networkv1.HTTPIngressRuleValue{
					Paths: []networkv1.HTTPIngressPath{
						{
							Path:     "/",
							PathType: &pt,
							Backend: networkv1.IngressBackend{
								Service: &networkv1.IngressServiceBackend{
									Name: route.Service,
									Port: servicePort,
								},
							},
						},
					},
				},
			},
		},
	}
	// check if any alternative names were provided and add them to the spec
	for _, alternativeName := range route.AlternativeNames {
		ingress.Spec.TLS[0].Hosts = append(ingress.Spec.TLS[0].Hosts, alternativeName)
		altName := networkv1.IngressRule{
			Host: alternativeName,
			IngressRuleValue: networkv1.IngressRuleValue{
				HTTP: &networkv1.HTTPIngressRuleValue{
					Paths: []networkv1.HTTPIngressPath{
						{
							Path:     "/",
							PathType: &pt,
							Backend: networkv1.IngressBackend{
								Service: &networkv1.IngressServiceBackend{
									Name: route.Service,
									Port: servicePort,
								},
							},
						},
					},
				},
			},
		}
		ingress.Spec.Rules = append(ingress.Spec.Rules, altName)
	}

	// @TODO: we should review this in the future when we stop doing `kubectl apply` in the builds :)
	// marshal the resulting ingress
	ingressBytes, _ := yaml.Marshal(ingress)
	// add the seperator to the template so that it can be `kubectl apply` in bulk as part
	// of the current build process
	separator := []byte("---\n")
	result := append(separator[:], ingressBytes[:]...)
	return result
}

// WriteTemplateFile writes the template to a file.
func WriteTemplateFile(templateOutputFile string, data []byte) {
	err := os.WriteFile(templateOutputFile, data, 0644)
	if err != nil {
		fmt.Println(err)
		return
	}
}
