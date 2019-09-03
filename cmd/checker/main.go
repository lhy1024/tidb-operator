package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/Masterminds/semver"
	"github.com/golang/glog"
	pdconfig "github.com/pingcap/pd/server"
	tidbconfig "github.com/pingcap/tidb/config"
	yaml "gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/helm/pkg/chartutil"
	"k8s.io/helm/pkg/engine"
	chartproto "k8s.io/helm/pkg/proto/hapi/chart"
	"k8s.io/helm/pkg/timeconv"
	tversion "k8s.io/helm/pkg/version"
)

var (
	chart        string
	values       string
	releaseName  string
	namespace    string
	chartVersion string
	kubeVersion  string = "v1.12.8"
)

func init() {
	flag.StringVar(&chart, "chart", "pingcap/tidb-cluster", "chart of tidb cluster")
	flag.StringVar(&values, "values", "values.yaml", "values file of tidb cluster chart")
	flag.StringVar(&releaseName, "name", "tidb-cluster", "release name")
	flag.StringVar(&namespace, "namespace", "default", "namespace of the release")
	flag.StringVar(&chartVersion, "chart-version", "v1.0.0", "chart version of tidb cluster")
	flag.Parse()
}

func main() {
	c, err := chartutil.Load(chart)
	if err != nil {
		glog.Fatalf("failed to load chart %s: %v", chart, err)
	}

	renderer := engine.New()
	caps := &chartutil.Capabilities{
		APIVersions:   chartutil.DefaultVersionSet,
		KubeVersion:   chartutil.DefaultKubeVersion,
		TillerVersion: tversion.GetVersionProto(),
	}

	valuesFile, err := os.Open(values)
	defer valuesFile.Close()
	if err != nil {
		glog.Fatalf("failed to open values file %s: %v", values, err)
	}
	bytes, err := ioutil.ReadAll(valuesFile)
	if err != nil {
		glog.Fatalf("failed to read file %s: %v", values, err)
	}
	// var data map[string]interface{}
	// if err := yaml.Unmarshal(bytes, &data); err != nil {
	// 	glog.Fatalf("failed to parse %s: %v", values, err)
	// }

	config := &chartproto.Config{Raw: string(bytes), Values: map[string]*chartproto.Value{}}
	// kubernetes version
	kv, err := semver.NewVersion(kubeVersion)
	if err != nil {
		glog.Fatalf("could not parse a kubernetes version: %v", err)
	}
	caps.KubeVersion.Major = fmt.Sprint(kv.Major())
	caps.KubeVersion.Minor = fmt.Sprint(kv.Minor())
	caps.KubeVersion.GitVersion = fmt.Sprintf("v%d.%d.0", kv.Major(), kv.Minor())

	options := chartutil.ReleaseOptions{
		Name:      releaseName,
		Time:      timeconv.Now(),
		Namespace: namespace,
	}
	vals, err := chartutil.ToRenderValuesCaps(c, config, options, caps)
	if err != nil {
		glog.Fatalf("failed to render: %v", err)
	}

	out, err := renderer.Render(c, vals)
	if err != nil {
		glog.Fatalf("failed to render chart %s: %v", c, err)
	}
	var tidbcfg *tidbconfig.Config
	var pdcfg *pdconfig.Config
	for k, v := range out {
		filename := filepath.Base(k)
		switch filename {
		case "tidb-configmap.yaml":
			tidbConfigMap := corev1.ConfigMap{}
			err = yaml.Unmarshal([]byte(v), &tidbConfigMap)
			if err != nil {
				glog.Fatalf("failed to unmarshal tidb configmap: %v", err)
			}
			if cfg, exist := tidbConfigMap.Data["config-file"]; exist {
				glog.Infof("TiDB config: %s", cfg)
				_, err = toml.Decode(cfg, &tidbcfg)
				if err != nil {
					glog.Fatalf("failed to decode tidb config: %v", err)
				}
			}
		case "tikv-configmap.yaml":
			tikvConfigMap := corev1.ConfigMap{}
			err = yaml.Unmarshal([]byte(v), &tikvConfigMap)
			if err != nil {
				glog.Fatalf("failed to unmarshal tidb configmap")
			}
			if cfg, exist := tikvConfigMap.Data["config-file"]; exist {
				glog.Infof("TiKV config: %s", cfg)
			}
		case "pd-configmap.yaml":
			pdConfigMap := corev1.ConfigMap{}
			err = yaml.Unmarshal([]byte(v), &pdConfigMap)
			if err != nil {
				glog.Fatalf("failed to unmarshal tidb configmap")
			}
			if cfg, exist := pdConfigMap.Data["config-file"]; exist {
				glog.Infof("PD config: %s", cfg)
				_, err = toml.Decode(cfg, &pdcfg)
				if err != nil {
					glog.Fatalf("failed to decode tidb config: %v", err)
				}
			}
		}
	}
	if tidbcfg == nil {
		glog.Fatalf("tidb configuration is empty")
	}
	if pdcfg == nil {
		glog.Fatalf("pd configuration is empty")
	}
}
