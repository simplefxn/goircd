package config

type Nats struct {
	Channels []NatsChannel `yaml:"channels"`
}

type NatsChannel struct {
	URL       string `yaml:"url"`
	Name      string `yaml:"name"`
	Direction string `yaml:"direction"`
	Topic     string `yaml:"topic"`
}
