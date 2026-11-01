package config

import (
	"reflect"
	"strings"
	"testing"
)

func TestProductionRejectsWildcardInCORSOriginList(t *testing.T) {
	defer withProdEnv(t)()
	for _, origins := range []string{"https://trusted.example,*", "*,https://trusted.example", "https://one.example, * ,https://two.example", " https://trusted.example ,\t*\t "} {
		CORSAllowOrigins = origins
		err := Validate()
		t.Logf("origins=%q rejected=%v", origins, err != nil)
		if err == nil || !strings.Contains(err.Error(), "CORS wildcard") {
			t.Errorf("production must reject wildcard entry in %q: %v", origins, err)
		}
	}
}

func TestCORSOriginsCompatibility(t *testing.T) {
	oldEnv, oldOrigins := Env, CORSAllowOrigins
	t.Cleanup(func() { Env, CORSAllowOrigins = oldEnv, oldOrigins })
	for _, tc := range []struct {
		name  string
		env   Environment
		input string
		want  []string
	}{
		{"disabled", EnvProduction, "", nil},
		{"whitespace disabled", EnvProduction, " \t ", nil},
		{"explicit list", EnvProduction, " https://trusted.example , https://other.example ", []string{"https://trusted.example", "https://other.example"}},
		{"development wildcard", EnvDevelopment, "https://trusted.example, *", []string{"https://trusted.example", "*"}},
		{"test wildcard", EnvTest, " * ", []string{"*"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			Env, CORSAllowOrigins = tc.env, tc.input
			got, err := CORSOrigins()
			if err != nil || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("origins=%v err=%v", got, err)
			}
		})
	}
}
