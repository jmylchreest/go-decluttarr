package config

import (
	"github.com/spf13/viper"
	"testing"
)

func TestTrackerPoliciesAndWhisparr(t *testing.T) {
	for _, policy := range []string{"keep", "skip", "remove", "obsolete_tag", "pause"} {
		t.Run(policy, func(t *testing.T) {
			v := viper.New()
			setDefaults(v)
			var c Config
			if err := v.Unmarshal(&c); err != nil {
				t.Fatal(err)
			}
			c.General.PrivateTrackerHandling = policy
			c.Instances.Whisparr = []InstanceConfig{{Name: "whisparr", URL: "http://localhost:6969", APIKey: "test"}}
			err := c.Validate()
			if policy == "pause" {
				if err == nil {
					t.Fatal("unimplemented policy accepted")
				}
			} else if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestNewIntegrationConfiguration(t *testing.T) {
	for _, kind := range []string{"sportarr", "bookshelf", "shelfarr"} {
		t.Run(kind, func(t *testing.T) {
			v := viper.New()
			setDefaults(v)
			v.Set("instances."+kind, []map[string]interface{}{{"name": kind, "url": "http://localhost:1234", "api_key": "test", "protected_categories": []string{"books"}}})
			var cfg Config
			if err := v.Unmarshal(&cfg); err != nil {
				t.Fatal(err)
			}
			if err := cfg.Validate(); err != nil {
				t.Fatal(err)
			}
			if kind == "shelfarr" {
				cfg.Instances.Shelfarr[0].ProtectedCategories = nil
				if cfg.Validate() == nil {
					t.Fatal("unprotected Shelfarr accepted")
				}
				cfg.Instances.Shelfarr[0].ProtectedCategories = []string{" "}
				if cfg.Validate() == nil {
					t.Fatal("blank category accepted")
				}
				cfg.Instances.Shelfarr[0].ProtectedCategories = []string{"books"}
			}
			cfg.Instances.Sonarr = []InstanceConfig{{Name: kind, URL: "http://localhost", APIKey: "test"}}
			if cfg.Validate() == nil {
				t.Fatal("duplicate name accepted")
			}
		})
	}
}
