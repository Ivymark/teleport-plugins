package main

import (
	"github.com/gravitational/teleport-plugins/lib"
	"github.com/gravitational/teleport-plugins/lib/logger"
	"github.com/gravitational/trace"
	"github.com/pelletier/go-toml"
)

// Config stores the full configuration for the teleport-slack plugin to run.
type Config struct {
	Teleport lib.TeleportConfig `toml:"teleport"`
	Slack    SlackConfig        `toml:"slack"`
	Log      logger.Config      `toml:"log"`
}

// SlackConfig holds Slack-specific configuration options.
type SlackConfig struct {
	Token      string   `toml:"token"`
	Recipients []string `toml:"recipients"`
	APIURL     string
}

const exampleConfig = `# Example slack plugin configuration TOML file

[teleport]
auth_server = "0.0.0.0:3025"                              # Teleport Auth Server GRPC API address

# tctl auth sign --format=file --auth-server=auth.example.com:3025 --user=access-plugin --out=auth --ttl=1h
identity = "/var/lib/teleport/plugins/slack/auth"         # Teleport certificate ("file" format)

# tctl auth sign --format=tls --auth-server=auth.example.com:3025 --user=access-plugin --out=auth --ttl=1h
# client_key = "/var/lib/teleport/plugins/slack/auth.key" # Teleport GRPC client secret key ("tls" format")
# client_crt = "/var/lib/teleport/plugins/slack/auth.crt" # Teleport GRPC client certificate ("tls" format")
# root_cas = "/var/lib/teleport/plugins/slack/auth.cas"   # Teleport cluster CA certs ("tls" format")

[slack]
token = "xoxb-11xx"                                 # Slack Bot OAuth token
# recipients = ["person@email.com","YYYYYYY"]       # Optional Slack Rooms 
                                                    # Can also set suggested_reviewers for each role

[log]
output = "stderr" # Logger output. Could be "stdout", "stderr" or "/var/lib/teleport/slack.log"
severity = "INFO" # Logger severity. Could be "INFO", "ERROR", "DEBUG" or "WARN".
`

// LoadConfig reads the config file, initializes a new Config struct object, and returns it.
// Optionally returns an error if the file is not readable, or if file format is invalid.
func LoadConfig(filepath string) (*Config, error) {
	t, err := toml.LoadFile(filepath)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	conf := &Config{}
	if err := t.Unmarshal(conf); err != nil {
		return nil, trace.Wrap(err)
	}
	if err := conf.CheckAndSetDefaults(); err != nil {
		return nil, trace.Wrap(err)
	}
	return conf, nil
}

// CheckAndSetDefaults checks the config struct for any logical errors, and sets default values
// if some values are missing.
// If critical values are missing and we can't set defaults for them — this will return an error.
func (c *Config) CheckAndSetDefaults() error {
	if c.Slack.Token == "" {
		return trace.BadParameter("missing required value slack.token")
	}
	if c.Log.Output == "" {
		c.Log.Output = "stderr"
	}
	if c.Log.Severity == "" {
		c.Log.Severity = "info"
	}
	return nil
}
