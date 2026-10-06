package pm

import "testing"

func TestValidatorsRejectLeadingDash(t *testing.T) {
	for _, name := range []string{"-g", "--global", "--working-dir=/etc", "-rf"} {
		if ValidateGenericPackageName(name) == nil {
			t.Errorf("ValidateGenericPackageName(%q) = nil, want error", name)
		}
		for _, id := range []ID{Npm, Yarn, Pip, Composer, Cargo, GoMod, Maven, Gradle} {
			if ValidatePackageName(name, id) == nil {
				t.Errorf("ValidatePackageName(%q, %s) = nil, want error", name, id)
			}
		}
	}
	if ValidateVersion("--registry=http://evil") == nil {
		t.Error("ValidateVersion accepted an option-looking version")
	}
}

func TestValidatorsStillAcceptRealNames(t *testing.T) {
	ok := map[ID]string{Npm: "@types/node", Pip: "requests", Composer: "monolog/monolog", Cargo: "serde", GoMod: "github.com/gin-gonic/gin", Maven: "com.google.guava:guava"}
	for id, name := range ok {
		if err := ValidatePackageName(name, id); err != nil {
			t.Errorf("ValidatePackageName(%q, %s) = %v", name, id, err)
		}
	}
	if err := ValidateVersion("1.2.3-beta.1"); err != nil {
		t.Error(err)
	}
}
func TestComposerNamesMustBeVendorPackage(t *testing.T) {
	for _, ok := range []string{"monolog/monolog", "symfony/http-kernel", "doctrine/dbal"} {
		if err := ValidatePackageName(ok, Composer); err != nil {
			t.Errorf("%q rejected: %v", ok, err)
		}
	}
	for _, bad := range []string{"monolog", "Monolog/Monolog", "a/b/c", "vendor/"} {
		if ValidatePackageName(bad, Composer) == nil {
			t.Errorf("%q accepted for composer", bad)
		}
	}
}

func TestRejectLeadingDash(t *testing.T) {
	if rejectLeadingDash("version", "1.0.0") != nil || rejectLeadingDash("version", "-1") == nil {
		t.Fatal("rejectLeadingDash must reject exactly the values that start with '-'")
	}
}
