package governance

import (
	"fmt"
	"text/template"

	"gopkg.in/yaml.v3"
)

type config struct {
	Version             string                `yaml:"version"`
	ExemptAssociations  []string              `yaml:"exemptAssociations"`
	ExemptActorSuffixes []string              `yaml:"exemptActorSuffixes"`
	PRPolicy            prPolicyConfig        `yaml:"prPolicy"`
	LabelGovernance     labelGovernanceConfig `yaml:"labelGovernance"`
	LabelInheritance    labelInheritanceConfig `yaml:"labelInheritance"`
	SlashCommands       slashCommandsConfig   `yaml:"slashCommands"`
}

type prPolicyConfig struct {
	BlockingLabels []string     `yaml:"blockingLabels"`
	NoLinkedIssue  action `yaml:"noLinkedIssue"`
	BlockedIssue   action `yaml:"blockedIssue"`
}

type action struct {
	AddLabels    []string `yaml:"addLabels"`
	RemoveLabels []string `yaml:"removeLabels"`
	Comment      string   `yaml:"comment"`
	Close        bool     `yaml:"close"`
}

type labelGovernanceConfig struct {
	Issue       []labelGroup `yaml:"issue"`
	PullRequest []labelGroup `yaml:"pullRequest"`
}

type labelGroup struct {
	Prefix   string   `yaml:"prefix"`
	Multiple bool     `yaml:"multiple"`
	Values   []string `yaml:"values"`
}

type labelInheritanceConfig struct {
	Prefixes []string `yaml:"prefixes"`
}

type slashCommandsConfig struct {
	Issue       map[string]commandDef `yaml:"issue"`
	PullRequest map[string]commandDef `yaml:"pullRequest"`
}

type commandDef struct {
	Description string `yaml:"description"`
	RequiresArg bool   `yaml:"requiresArg"`
	action      `yaml:",inline"`
}

func parseConfig(data []byte) (*config, error) {
	cfg := &config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("error parsing governance config: %w", err)
	}
	if err := validateConfig(cfg); err != nil {
		return nil, fmt.Errorf("error validating governance config: %w", err)
	}
	return cfg, nil
}

func validateConfig(cfg *config) error {
	if err := validateTemplate(
		"prPolicy.noLinkedIssue.comment",
		cfg.PRPolicy.NoLinkedIssue.Comment,
	); err != nil {
		return err
	}
	if err := validateTemplate(
		"prPolicy.blockedIssue.comment",
		cfg.PRPolicy.BlockedIssue.Comment,
	); err != nil {
		return err
	}
	for name, cmd := range cfg.SlashCommands.Issue {
		if err := validateTemplate(
			fmt.Sprintf("slashCommands.issue.%s.comment", name),
			cmd.Comment,
		); err != nil {
			return err
		}
	}
	for name, cmd := range cfg.SlashCommands.PullRequest {
		if err := validateTemplate(
			fmt.Sprintf("slashCommands.pullRequest.%s.comment", name),
			cmd.Comment,
		); err != nil {
			return err
		}
	}
	return nil
}

func validateTemplate(name, tmpl string) error {
	if tmpl == "" {
		return nil
	}
	if _, err := template.New(name).Parse(tmpl); err != nil {
		return fmt.Errorf("error parsing template %q: %w", name, err)
	}
	return nil
}
