package config

import (
	"fmt"
	"net"
	"regexp"
	"strings"
	"time"

	"github.com/miekg/dns"
	"github.com/paulc/dinosaur-dns/blocklist"
	"github.com/paulc/dinosaur-dns/logger"
	"github.com/paulc/dinosaur-dns/util"
)

type UserConfig struct {
	Listen             []string `json:"listen"`
	Upstream           []string `json:"upstream"`
	Acl                []string `json:"acl"`
	Block              []string `json:"block"`
	BlockDelete        []string `json:"block-delete"`
	Blocklist          []string `json:"blocklist"`
	BlocklistAAAA      []string `json:"blocklist-aaaa"`
	BlocklistFromHosts []string `json:"blocklist-from-hosts"`
	Local              []string `json:"local"`
	Localzone          []string `json:"localzone"`
	Dns64              bool     `json:"dns64"`
	Dns64Prefix        string   `json:"dns64-prefix"`
	Api                bool     `json:"api"`
	ApiBind            string   `json:"api-bind"`
	Refresh            bool     `json:"refresh"`
	RefreshInterval    string   `json:"refresh-interval"`
	Debug              bool     `json:"debug"`
	Syslog             bool     `json:"syslog"`
	Discard            bool     `json:"discard"`
}

func NewUserConfig() *UserConfig {
	return &UserConfig{
		Listen:             make([]string, 0),
		Upstream:           make([]string, 0),
		Acl:                make([]string, 0),
		Block:              make([]string, 0),
		BlockDelete:        make([]string, 0),
		Blocklist:          make([]string, 0),
		BlocklistAAAA:      make([]string, 0),
		BlocklistFromHosts: make([]string, 0),
		Local:              make([]string, 0),
		Localzone:          make([]string, 0),
	}
}

// Generate config from user options
func (user_config *UserConfig) GetProxyConfig(config *ProxyConfig) error {

	// Listen addresses
	for _, v := range user_config.Listen {
		if addrs, err := util.ParseAddr(v, 53); err != nil {
			return err
		} else {
			for _, v := range addrs {
				config.ListenAddr = append(config.ListenAddr, v)
			}
		}
	}

	// Upstream resolvers
	for _, v := range user_config.Upstream {
		// Add default port if not specified for non DoH
		if !strings.HasPrefix(v, "https://") && !regexp.MustCompile(`:\d+$`).MatchString(v) {
			v += ":53"
		}
		config.Upstream = append(config.Upstream, v)
	}

	// Generate blocklist
	if err := user_config.UpdateBlockList(config.BlockList); err != nil {
		return err
	}

	// Local RRs
	for _, v := range user_config.Local {
		if err := config.Cache.AddRR(v, true); err != nil {
			return err
		}
	}

	// Local RR file/url
	for _, v := range user_config.Localzone {
		if _, err := util.URLReader(v, func(line string) error { return config.Cache.AddRR(line, true) }); err != nil {
			return err
		}
	}

	// Access control list
	for _, v := range user_config.Acl {
		_, cidr, err := net.ParseCIDR(v)
		if err != nil {
			return fmt.Errorf("ACL Error (%s): %s", v, err)
		}
		config.Acl = append(config.Acl, *cidr)
	}

	// DNS64
	if user_config.Dns64 {
		config.Dns64 = true
		if user_config.Dns64Prefix != "" {
			_, ipv6Net, err := net.ParseCIDR(user_config.Dns64Prefix)
			if err != nil {
				return fmt.Errorf("Dns64 Prefix Error (%s): %s", user_config.Dns64Prefix, err)
			}
			ones, bits := ipv6Net.Mask.Size()
			if ones != 96 || bits != 128 {
				return fmt.Errorf("Dns64 Prefix Error (%s): Invalid prefix", user_config.Dns64Prefix)
			}
			config.Dns64Prefix = *ipv6Net
		}
	}

	// API
	config.Api = user_config.Api
	if user_config.ApiBind != "" {
		config.ApiBind = user_config.ApiBind
	}

	// Refresh Blocklist
	config.Refresh = user_config.Refresh
	if user_config.RefreshInterval != "" {
		duration, err := time.ParseDuration(user_config.RefreshInterval)
		if err != nil {
			return err
		}
		if duration < time.Second {
			return fmt.Errorf("Invalid duration: %s", duration)
		}
		config.RefreshInterval = duration
	}

	// Logging
	if user_config.Discard {
		config.Log = logger.New(logger.NewDiscard(false))
	} else {
		if user_config.Syslog {
			config.Log = logger.New(logger.NewSyslog(user_config.Debug))
		} else {
			config.Log = logger.New(logger.NewStderr(user_config.Debug))
		}
	}

	// Add reference to UserConfig
	config.UserConfig = user_config
	return nil
}

func (user_config *UserConfig) UpdateBlockList(bl *blocklist.BlockList) error {

	// Block entries
	for _, v := range user_config.Block {
		if err := bl.AddEntry(v, dns.TypeANY); err != nil {
			return err
		}
	}

	// Blocklist file/url
	for _, v := range user_config.Blocklist {
		if _, err := util.URLReader(v, blocklist.MakeBlockListReaderf(bl, dns.TypeANY)); err != nil {
			return err
		}
	}

	// Blocklist file/url (AAAA)
	for _, v := range user_config.BlocklistAAAA {
		if _, err := util.URLReader(v, blocklist.MakeBlockListReaderf(bl, dns.TypeAAAA)); err != nil {
			return err
		}
	}

	// Blocklist hosts file
	for _, v := range user_config.BlocklistFromHosts {
		if _, err := util.URLReader(v, blocklist.MakeBlockListHostsReaderf(bl)); err != nil {
			return err
		}
	}

	// Delete blocklist entries last
	for _, v := range user_config.BlockDelete {
		if _, err := bl.DeleteEntry(v, dns.TypeANY); err != nil {
			return err
		}
	}

	return nil
}
