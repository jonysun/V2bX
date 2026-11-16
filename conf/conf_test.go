package conf

import (
	"testing"
)

func TestConf_LoadFromPath(t *testing.T) {
	c := New()
	t.Log(c.LoadFromPath("../example/config.json"), c.NodeConfig)
}

func TestConf_Watch(t *testing.T) {
	c := New()
	if err := c.Watch(t.TempDir()+"/missing.json", "", "", func() {}); err == nil {
		t.Fatal("Watch accepted a missing configuration file")
	}
}
