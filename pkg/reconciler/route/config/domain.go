/*
Copyright 2018 The Knative Authors

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

package config

import (
	"strings"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/yaml"

	netpkg "knative.dev/networking/pkg"
	"knative.dev/pkg/configmap"
	"knative.dev/pkg/network"
	"knative.dev/serving/pkg/apis/serving"
)

const (
	// DomainConfigName is the config map name for the domain configuration.
	DomainConfigName = "config-domain"
	// DefaultDomain holds the domain that Route's live under by default
	// when no label selector-based options apply.
	DefaultDomain = "example.com"
)

// LabelSelector represents map of {key,value} pairs. A single {key,value} in the
// map is equivalent to a requirement key == value. The requirements are ANDed.
type LabelSelector struct {
	Selector map[string]string `json:"selector,omitempty"`
}

func (s *LabelSelector) specificity() int {
	if s == nil {
		return 0
	}
	return len(s.Selector)
}

// Matches returns whether the given labels meet the requirement of the selector.
func (s *LabelSelector) Matches(labels map[string]string) bool {
	if s == nil {
		return true
	}
	for label, expectedValue := range s.Selector {
		value, ok := labels[label]
		if !ok || expectedValue != value {
			return false
		}
	}
	return true
}

// Domain maps domains to routes by matching the domain's
// label selectors to the route's labels.
type Domain struct {
	// Domains map from domain to a set of options including a label selector.
	Domains map[string]DomainConfig
}

// The configuration of one domain
type DomainConfig struct {
	// The label selector for the domain. If a route has labels matching a particular selector, it
	// will use the corresponding domain. If multiple selectors match, we choose the most specific
	// selector.
	Selector *LabelSelector `json:"selector,omitempty"`
	// If true, the domain will have a wildcard TLS certificate generated.
	Wildcard bool `json:"wildcard"`
}

// Internal only representation of domain config for unmarshalling, allows backwards compatibility
type domainInternalConfig struct {
	Selector map[string]string `json:"selector,omitempty"`
	Wildcard bool              `json:"wildcard"`
}

// NewDomainFromConfigMap creates a Domain from the supplied ConfigMap
func NewDomainFromConfigMap(configMap *corev1.ConfigMap) (*Domain, error) {
	c := Domain{Domains: map[string]DomainConfig{}}
	hasDefault := false
	for k, v := range configMap.Data {
		if k == configmap.ExampleKey {
			continue
		}
		internalConfig := domainInternalConfig{}
		err := yaml.Unmarshal([]byte(v), &internalConfig)
		if err != nil {
			return nil, err
		}
		if len(internalConfig.Selector) == 0 {
			hasDefault = true
			internalConfig.Wildcard = true
		}
		c.Domains[k] = DomainConfig{
			Selector: &LabelSelector{Selector: internalConfig.Selector},
			Wildcard: internalConfig.Wildcard,
		}
	}
	if !hasDefault {
		c.Domains[DefaultDomain] = DomainConfig{Selector: &LabelSelector{}, Wildcard: true}
	}
	return &c, nil
}

// LookupDomainForLabels returns a domain given a set of labels.
// Since we reject configuration without a default domain, this should
// always return a value.
func (c *Domain) LookupDomainForLabels(labels map[string]string) string {
	domain := ""
	specificity := -1
	// If we see VisibilityLabelKey sets with VisibilityClusterLocal, that
	// will take precedence and the route will get a Cluster's Domain Name.
	if labels[netpkg.VisibilityLabelKey] == serving.VisibilityClusterLocal {
		return "svc." + network.GetClusterDomainName()
	}
	for k, v := range c.Domains {

		// Ignore if selector doesn't match, or decrease the specificity.
		if !v.Selector.Matches(labels) || v.Selector.specificity() < specificity {
			continue
		}
		if v.Selector.specificity() > specificity || strings.Compare(k, domain) < 0 {
			domain = k
			specificity = v.Selector.specificity()
		}
	}

	return domain
}
