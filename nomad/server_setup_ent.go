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

// FeatureCheckPreemption feature checks for preemption
// It exists so it can be used  in an OSS context without requiring
// the OSS binary to include license pkg.
func (es *EnterpriseState) FeatureCheckPreemption() error {
	return es.FeatureCheck(license.FeaturePreemption, true)
}

func (es *EnterpriseState) Features() uint64 {
	return uint64(es.licenseWatcher.Features())
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

	licenseWatcher, err := NewLicenseWatcher(s.logger)
	if err != nil {
		return fmt.Errorf("failed to create a new license watcher: %w", err)
	}
	s.EnterpriseState.licenseWatcher = licenseWatcher
	s.EnterpriseState.licenseWatcher.start(s.shutdownCtx, s.State(), s.Shutdown)
	return nil
}

// startEnterpriseBackground starts the Enterprise specific workers
func (s *Server) startEnterpriseBackground() {
	// Garbage collect Sentinel policies if enabled
	if s.config.ACLEnabled {
		go s.gcSentinelPolicies(s.shutdownCh)
	}
}
