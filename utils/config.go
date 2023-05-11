package utils

import (
	"log"
	"os"
	"path"

	"github.com/spf13/viper"
)

// Config is the cofiguration for the application
type Config struct {
	RootPath   string   `mapstructure:"root_path"`  // Root path of the notes.
	Editor     string   `mapstructure:"editor"`     // Editor to open the notes with
	Extensions []string `mapstructure:"extensions"` // Extensions of notes to be indexed
}

// NewConfig returns a new Config object by reading from the config file
func NewConfig() *Config {
	homedir, _ := os.UserHomeDir()
	configPath := path.Join(homedir, "/.config/notes_search/config.yaml")
	viper.SetConfigFile(configPath)

	viper.SetDefault("extensions", []string{".md"})

	if err := viper.ReadInConfig(); err != nil {
		log.Fatal("failed to read config file", err)
	}

	config := &Config{}
	err := viper.Unmarshal(config)
	if err != nil {
		log.Fatal("unable to parse the config file", err)
	}

	return config
}
