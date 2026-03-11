package config

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server     ServerConfig   `yaml:"server"`
	Categories []string       `yaml:"categories"`
	Database   DatabaseConfig `yaml:"database"`
	Log        LogConfig      `yaml:"log"`
	AI         AIConfig       `yaml:"ai"`
	SiYuan     SiYuanConfig   `yaml:"siyuan"`
	PDF        PDFConfig      `yaml:"pdf"`
}

type ServerConfig struct {
	Port int    `yaml:"port"`
	Host string `yaml:"host"`
}

type DatabaseConfig struct {
	Filename string `yaml:"filename"`
}

type LogConfig struct {
	Level string `yaml:"level"`
}

type AIConfig struct {
	APIKey  string `yaml:"api_key"`
	BaseURL string `yaml:"base_url"`
	Model   string `yaml:"model"`
}

type SiYuanConfig struct {
	APIURL     string `yaml:"api_url"`
	NotebookID string `yaml:"notebook_id"`
}

type PDFConfig struct {
	FontPath string `yaml:"font_path"`
}

func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Port: 8080,
			Host: "0.0.0.0",
		},
		Categories: []string{},
		Database:   DatabaseConfig{Filename: "tickets.db"},
		Log:        LogConfig{Level: "info"},
		SiYuan: SiYuanConfig{
			APIURL:     "http://127.0.0.1:6806",
			NotebookID: "",
		},
		PDF: PDFConfig{
			FontPath: DetectFontPath(),
		},
	}
}

// DetectFontPath 根据操作系统检测默认中文字体路径
func DetectFontPath() string {
	fonts := []string{}

	// 根据操作系统选择可能的字体路径
	switch runtime.GOOS {
	case "windows":
		fonts = []string{
			"C:/Windows/Fonts/simhei.ttf", // 黑体
			"C:/Windows/Fonts/msyh.ttc",   // 微软雅黑
			"C:/Windows/Fonts/simsun.ttc", // 宋体
		}
	case "darwin":
		fonts = []string{
			"/System/Library/Fonts/PingFang.ttc",
			"/Library/Fonts/Arial Unicode.ttf",
		}
	default: // Linux
		fonts = []string{
			"/usr/share/fonts/fonts-gb/GB_HT_GB18030.ttf",
			"/usr/share/fonts/truetype/wqy/wqy-microhei.ttc",
			"/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc",
		}
	}

	for _, font := range fonts {
		if _, err := os.Stat(font); err == nil {
			return font
		}
	}
	return ""
}

func Load() (*Config, error) {
	cfg := DefaultConfig()
	paths := []string{
		"./config.yaml",
		filepath.Join(os.Getenv("HOME"), ".tix", "config.yaml"),
	}
	for _, path := range paths {
		if data, err := os.ReadFile(path); err == nil {
			if err := yaml.Unmarshal(data, cfg); err != nil {
				return nil, err
			}
			break
		}
	}
	return cfg, nil
}

func (c *Config) IsValidCategory(cat string) bool {
	return slices.Contains(c.Categories, cat)
}

// Save 保存配置到文件
func Save(cfg *Config) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	configDir := filepath.Join(home, ".tix")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return err
	}

	configPath := filepath.Join(configDir, "config.yaml")
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, data, 0644)
}
