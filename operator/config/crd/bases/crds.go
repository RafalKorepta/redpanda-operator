// Copyright 2024 Redpanda Data, Inc.
//
// Use of this software is governed by the Business Source License
// included in the file licenses/BSL.md
//
// As of the Change Date specified in that file, in accordance with
// the Business Source License, use of this software will be governed
// by the Apache License, Version 2.0

// Package crds provide programmatic access to the CRDs generated by
// controller-gen.
package crds

import (
	"embed"
	"io/fs"

	"github.com/cockroachdb/errors"
	"github.com/redpanda-data/helm-charts/pkg/kube"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var (
	//go:embed *.yaml
	//go:embed toolkit.fluxcd.io/*.yaml
	crdFS embed.FS

	crds   []*apiextensionsv1.CustomResourceDefinition
	byName map[string]*apiextensionsv1.CustomResourceDefinition
)

func init() {
	scheme := runtime.NewScheme()
	must(apiextensionsv1.AddToScheme(scheme))

	byName = map[string]*apiextensionsv1.CustomResourceDefinition{}

	must(fs.WalkDir(crdFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		data, err := fs.ReadFile(crdFS, path)
		if err != nil {
			return err
		}

		objs, err := kube.DecodeYAML(data, scheme)
		if err != nil {
			return err
		}

		for _, obj := range objs {
			crd := obj.(*apiextensionsv1.CustomResourceDefinition)

			crds = append(crds, crd)
			byName[crd.Name] = crd
		}

		return nil
	}))
}

func ByName(name string) (*apiextensionsv1.CustomResourceDefinition, error) {
	crd, ok := byName[name]
	if !ok {
		return nil, errors.Newf("no such CRD %q", name)
	}
	return crd, nil
}

func All() []*apiextensionsv1.CustomResourceDefinition {
	ret := make([]*apiextensionsv1.CustomResourceDefinition, len(crds))

	for i, crd := range crds {
		ret[i] = crd.DeepCopy()
	}

	return ret
}

// Redpanda returns the Redpanda CustomResourceDefinition.
func Redpanda() *apiextensionsv1.CustomResourceDefinition {
	return mustT(ByName("redpandas.cluster.redpanda.com"))
}

// Topic returns the Redpanda CustomResourceDefinition.
func Topic() *apiextensionsv1.CustomResourceDefinition {
	return mustT(ByName("topics.cluster.redpanda.com"))
}

// Topic returns the Redpanda CustomResourceDefinition.
func User() *apiextensionsv1.CustomResourceDefinition {
	return mustT(ByName("users.cluster.redpanda.com"))
}

func mustT[T any](r T, err error) T {
	must(err)
	return r
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}