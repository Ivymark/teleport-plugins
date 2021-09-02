package main

import (
	_ "embed"

	"github.com/gravitational/teleport-plugins/lib"
	"github.com/gravitational/teleport-plugins/lib/logger"
	"github.com/gravitational/trace"
	"github.com/pelletier/go-toml"
)

// DeliveryConfig represents email recipients config
type DeliveryConfig struct {
	Sender     string
	Recipients []string
}

// MailgunConfig holds Mailgun-specific configuration options.
type MailgunConfig struct {
	Domain     string
	PrivateKey string `toml:"private_key"`
}

// SMTPConfig is SMTP-specific configuration options
type SMTPConfig struct {
	Host     string
	Port     int
	Username string
	Password string
}

// Config stores the full configuration for the teleport-email plugin to run.
type Config struct {
	Teleport lib.TeleportConfig `toml:"teleport"`
	Mailgun  *MailgunConfig     `toml:"mailgun"`
	SMTP     *SMTPConfig        `toml:"smtp"`
	Delivery DeliveryConfig     `toml:"delivery"`
	Log      logger.Config      `toml:"log"`
}

const exampleConfig = `# Example email plugin configuration TOML file

[teleport]
auth_server = "0.0.0.0:3025"                              # Teleport Auth Server GRPC API address

# tctl auth sign --format=file --auth-server=auth.example.com:3025 --user=access-plugin --out=auth --ttl=1h
identity = "/var/lib/teleport/plugins/email/auth"         # Teleport certificate ("file" format)

# tctl auth sign --format=tls --auth-server=auth.example.com:3025 --user=access-plugin --out=auth --ttl=1h
# client_key = "/var/lib/teleport/plugins/email/auth.key" # Teleport GRPC client secret key ("tls" format")
# client_crt = "/var/lib/teleport/plugins/email/auth.crt" # Teleport GRPC client certificate ("tls" format")
# root_cas = "/var/lib/teleport/plugins/email/auth.cas"   # Teleport cluster CA certs ("tls" format")

[mailgun]
domain = "your-domain-name"
private_key = "xoxb-11xx"

[smtp]
host = "smtp.gmail.com"
port = 587
username = "username@gmail.com"
password = ""

[users]
sender = "noreply@example.com"    # From: email address
recipients = ["person@gmail.com"] # These recipients will receive all review requests

[log]
output = "stderr" # Logger output. Could be "stdout", "stderr" or "/var/lib/teleport/email.log"
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
	if c.Log.Output == "" {
		c.Log.Output = "stderr"
	}
	if c.Log.Severity == "" {
		c.Log.Severity = "info"
	}

	// Validate emails in user aliases
	for _, e := range c.Delivery.Recipients {
		if !lib.IsEmail(e) {
			return trace.BadParameter("Invalid email address %v in users.recipients", e)
		}
	}

	// Validate transport settings
	if c.SMTP == nil && c.Mailgun == nil {
		return trace.BadParameter("Provide either [mailgun] or [smtp] sections to work with plugin")
	}

	if c.Mailgun != nil {
		if c.Mailgun.Domain == "" || c.Mailgun.PrivateKey == "" {
			return trace.BadParameter("Please, provide mailgun.domain and mailgun.private_key to use Mailgun")
		}
	}

	if c.SMTP != nil {
		if c.SMTP.Host == "" {
			return trace.BadParameter("Please, provide smtp.host to use SMTP")
		}

		if c.SMTP.Port == 0 {
			c.SMTP.Port = 587
		}

		if c.SMTP.Username == "" {
			return trace.BadParameter("Please, provide smtp.username to use SMTP")
		}
	}

	return nil
}
