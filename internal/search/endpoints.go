package search

// Registry base URLs. They are variables so tests (and SetEndpoints) can
// point them at an httptest server; production code never changes them.
var (
	npmRegistryURL = NpmRegistryURL
	pypiURL        = PyPIURL
	packagistURL   = PackagistURL
	cratesAPIURL   = CratesIOURL
	cratesIndexURL = CratesIndexURL
	mavenSearchURL = MavenSearchURL
)

// CratesIndexURL is the crates.io sparse index (served from a CDN).
const CratesIndexURL = "https://index.crates.io"

// Endpoints overrides registry base URLs. Empty fields keep the current value.
type Endpoints struct {
	Npm         string // default https://registry.npmjs.org
	PyPI        string // default https://pypi.org/pypi
	Packagist   string // default https://packagist.org
	CratesAPI   string // default https://crates.io/api/v1
	CratesIndex string // default https://index.crates.io
	MavenSearch string // default https://central.sonatype.com/solrsearch/select
}

// SetEndpoints points registry lookups at other base URLs and returns a
// function that restores the previous ones. It exists for tests in other
// packages (fake registries via httptest); it is not safe to call while
// lookups are running.
func SetEndpoints(e Endpoints) (restore func()) {
	old := Endpoints{npmRegistryURL, pypiURL, packagistURL, cratesAPIURL, cratesIndexURL, mavenSearchURL}
	set := func(dst *string, v string) {
		if v != "" {
			*dst = v
		}
	}
	set(&npmRegistryURL, e.Npm)
	set(&pypiURL, e.PyPI)
	set(&packagistURL, e.Packagist)
	set(&cratesAPIURL, e.CratesAPI)
	set(&cratesIndexURL, e.CratesIndex)
	set(&mavenSearchURL, e.MavenSearch)
	return func() {
		npmRegistryURL, pypiURL, packagistURL = old.Npm, old.PyPI, old.Packagist
		cratesAPIURL, cratesIndexURL, mavenSearchURL = old.CratesAPI, old.CratesIndex, old.MavenSearch
	}
}

// SetCacheDir replaces the on-disk lookup cache directory ("" disables the
// cache) and returns a function that restores the previous one. For tests in
// other packages; not safe to call while lookups are running.
func SetCacheDir(dir string) (restore func()) {
	old := lookupCacheDir
	lookupCacheDir = dir
	return func() { lookupCacheDir = old }
}
