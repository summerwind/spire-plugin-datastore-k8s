package main

import (
	"errors"
)

type PluginConfig struct {
	KubeConfig string `hcl:"kubeconfig,omitempty"`
	Namespace  string `hcl:"namespace"`
}

func (p *PluginConfig) Validate() error {
	if p.Namespace == "" {
		return errors.New("namespace must be specified")
	}
	return nil
}
