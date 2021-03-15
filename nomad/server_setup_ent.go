// +build ent

package nomad

import (
	"fmt"

	"github.com/hashicorp/nomad-licensing/license"
	"github.com/hashicorp/sentinel/sentinel"
)

type EnterpriseState struct {
	// sentinel is a shared instance of the policy engine
	sentinel *sentinel.Sentinel

	//licenseWatcher is used to manage the lifecycle for enterprise licenses
	licenseWatcher *LicenseWatcher
}

func (es *EnterpriseState) FeatureCheck(feature license.Features, emitLog bool) error {
	if es.licenseWatcher == nil {
		// everything is licensed while the watcher starts up
		return nil
	}

	return es.licenseWatcher.FeatureCheck(feature, emitLog)
}

func (es *EnterpriseState) SetLicense(blob string, force bool) error {
	if es.licenseWatcher == nil {
		return fmt.Errorf("license watcher unable to set license")
	}

	return es.licenseWatcher.SetLicense(blob, force)
}

func (es *EnterpriseState) Features() uint64 {
	return uint64(es.licenseWatcher.Features())
}

// License returns the current license
func (es *EnterpriseState) License() *license.License {
	if es.licenseWatcher == nil {
		// everything is licensed while the watcher starts up
		return nil
	}
	return es.licenseWatcher.License()
}

func (es *EnterpriseState) ReloadLicense(cfg *Config) error {
	if es.licenseWatcher == nil {
		return nil
	}
	licenseConfig := &LicenseConfig{
		LicenseEnvBytes: cfg.LicenseFileEnv,
		LicensePath:     cfg.LicenseFilePath,
	}
	return es.licenseWatcher.Reload(licenseConfig)
}

// setupEnterprise is used for Enterprise specific setup
func (s *Server) setupEnterprise(config *Config) error {
	// Enable the standard lib, except the HTTP import.
	stdlib := sentinel.ImportsMap(sentinel.StdImports())
	stdlib.Blacklist([]string{"http"})

	// Setup the sentinel configuration
	sentConf := &sentinel.Config{
		Imports: stdlib,
	}
	if config.SentinelConfig != nil {
		for _, sImport := range config.SentinelConfig.Imports {
			sentConf.Imports[sImport.Name] = &sentinel.Import{
				Path: sImport.Path,
				Args: sImport.Args,
			}
			s.logger.Named("sentinel").Debug("enabling imports", "name", sImport.Name, "path", sImport.Path)
		}
	}

	// Create the Sentinel instance based on the configuration
	s.sentinel = sentinel.New(sentConf)

	s.setupEnterpriseAutopilot(config)

	// AdditionalPubKeys may be set prior to this step, mainly in tests
	additionalPubKeys := config.LicenseConfig.AdditionalPubKeys
	// Set License config options
	config.LicenseConfig = &LicenseConfig{
		AdditionalPubKeys:     additionalPubKeys,
		InitTmpLicenseBarrier: s.initTmpLicenseBarrier,
		LicenseEnvBytes:       config.LicenseEnv,
		LicensePath:           config.LicensePath,
		Logger:                s.logger,
		PropagateFn:           s.propagateLicense,
		ShutdownCallback:      config.AgentShutdown,
		StateStore:            s.State,
	}

	licenseWatcher, err := NewLicenseWatcher(config.LicenseConfig)
	if err != nil {
		return fmt.Errorf("failed to create a new license watcher: %w", err)
	}
	s.EnterpriseState.licenseWatcher = licenseWatcher
	return nil
}

// startEnterpriseBackground starts the Enterprise specific workers
func (s *Server) startEnterpriseBackground() {
	// Garbage collect Sentinel policies if enabled
	if s.config.ACLEnabled {
		go s.gcSentinelPolicies(s.shutdownCh)
	}

	s.EnterpriseState.licenseWatcher.start(s.shutdownCtx)
}

func (s *Server) entVaultDelegate() *VaultEntDelegate {
	return &VaultEntDelegate{
		featureChecker: &s.EnterpriseState,
		l:              s.logger,
	}
}
