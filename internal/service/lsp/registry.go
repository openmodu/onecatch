package lsp

import (
	"os/exec"
	"path/filepath"
	"strings"
)

type serverCandidate struct {
	command string
	args    []string
}

type serverDefinition struct {
	id          string
	name        string
	languages   map[string]string
	filenames   map[string]string
	candidates  []serverCandidate
	installHint string
}

type resolvedServer struct {
	definition *serverDefinition
	candidate  serverCandidate
	binary     string
	languageID string
}

// serverRegistry is intentionally explicit. LSP does not define a discovery
// protocol, so OneCatch recognizes file types and probes conventional server
// executables in the project and on PATH without starting or installing them.
var serverRegistry = []*serverDefinition{
	server("ada", "Ada Language Server", languages("ada", "ada"), nil, candidates(command("ada_language_server")), "Install Ada Language Server and add ada_language_server to PATH."),
	server("bash", "Bash Language Server", languages("sh,bash,zsh,ksh", "shellscript"), nil, candidates(commandArgs("bash-language-server", "start")), "npm install -g bash-language-server"),
	server("clangd", "clangd", languageMap(map[string]string{"c": "c", "h": "cpp", "cc": "cpp", "cpp": "cpp", "cxx": "cpp", "hh": "cpp", "hpp": "cpp", "hxx": "cpp", "inl": "cpp", "m": "objective-c", "mm": "objective-cpp"}), nil, candidates(command("clangd")), "Install clangd with LLVM and add it to PATH."),
	server("clojure", "Clojure LSP", languages("clj,cljs,cljc,edn", "clojure"), nil, candidates(command("clojure-lsp")), "Install clojure-lsp and add it to PATH."),
	server("cmake", "CMake Language Server", languages("cmake", "cmake"), filenameMap(map[string]string{"cmakelists.txt": "cmake"}), candidates(command("cmake-language-server")), "Install cmake-language-server and add it to PATH."),
	server("css", "CSS Language Server", languageMap(map[string]string{"css": "css", "scss": "scss", "less": "less"}), nil, candidates(commandArgs("vscode-css-language-server", "--stdio")), "npm install -g vscode-langservers-extracted"),
	server("csharp", "C# Language Server", languages("cs,csx", "csharp"), nil, candidates(command("csharp-ls"), commandArgs("omnisharp", "-lsp")), "Install csharp-ls or OmniSharp and add it to PATH."),
	server("dart", "Dart Language Server", languages("dart", "dart"), nil, candidates(commandArgs("dart", "language-server", "--protocol=lsp")), "Install the Dart SDK and add dart to PATH."),
	server("docker", "Docker Language Server", languages("dockerfile", "dockerfile"), filenameMap(map[string]string{"containerfile": "dockerfile", "dockerfile": "dockerfile"}), candidates(commandArgs("docker-langserver", "--stdio")), "npm install -g dockerfile-language-server-nodejs"),
	server("elixir", "Elixir Language Server", languageMap(map[string]string{"ex": "elixir", "exs": "elixir", "heex": "phoenix-heex"}), nil, candidates(command("elixir-ls"), command("language_server.sh")), "Install ElixirLS and add its launcher to PATH."),
	server("erlang", "Erlang LS", languages("erl,hrl", "erlang"), nil, candidates(command("erlang_ls")), "Install erlang_ls and add it to PATH."),
	server("go", "gopls", languages("go", "go"), filenameMap(map[string]string{"go.mod": "go.mod", "go.sum": "go.sum", "go.work": "go.work"}), candidates(commandArgs("gopls", "serve")), "go install golang.org/x/tools/gopls@latest"),
	server("graphql", "GraphQL Language Server", languages("graphql,gql", "graphql"), nil, candidates(commandArgs("graphql-lsp", "server", "-m", "stream")), "npm install -g graphql-language-service-cli"),
	server("groovy", "Groovy Language Server", languages("groovy,gvy,gy,gdsl,gradle", "groovy"), nil, candidates(command("groovy-language-server")), "Install groovy-language-server and add it to PATH."),
	server("haskell", "Haskell Language Server", languages("hs,lhs", "haskell"), nil, candidates(commandArgs("haskell-language-server-wrapper", "--lsp"), commandArgs("haskell-language-server", "--lsp")), "Install Haskell Language Server and add it to PATH."),
	server("html", "HTML Language Server", languages("html,htm", "html"), nil, candidates(commandArgs("vscode-html-language-server", "--stdio")), "npm install -g vscode-langservers-extracted"),
	server("java", "Eclipse JDT Language Server", languages("java", "java"), nil, candidates(command("jdtls")), "Install Eclipse JDT Language Server and add jdtls to PATH."),
	server("json", "JSON Language Server", languageMap(map[string]string{"json": "json", "jsonc": "jsonc"}), nil, candidates(commandArgs("vscode-json-language-server", "--stdio")), "npm install -g vscode-langservers-extracted"),
	server("kotlin", "Kotlin Language Server", languageMap(map[string]string{"kt": "kotlin", "kts": "kotlin"}), nil, candidates(command("kotlin-language-server")), "Install kotlin-language-server and add it to PATH."),
	server("latex", "TexLab", languageMap(map[string]string{"tex": "latex", "bib": "bibtex"}), nil, candidates(command("texlab")), "Install texlab and add it to PATH."),
	server("lua", "Lua Language Server", languages("lua", "lua"), nil, candidates(command("lua-language-server")), "Install lua-language-server and add it to PATH."),
	server("markdown", "Marksman", languages("md,markdown,mdx", "markdown"), nil, candidates(commandArgs("marksman", "server")), "Install marksman and add it to PATH."),
	server("metals", "Metals", languages("scala,sc,sbt", "scala"), nil, candidates(command("metals")), "Install Metals and add it to PATH."),
	server("nim", "Nim Language Server", languages("nim,nims,nimble", "nim"), nil, candidates(command("nimlangserver")), "Install nimlangserver and add it to PATH."),
	server("nix", "Nix Language Server", languages("nix", "nix"), nil, candidates(command("nil"), command("nixd")), "Install nil or nixd and add it to PATH."),
	server("ocaml", "OCaml LSP", languageMap(map[string]string{"ml": "ocaml", "mli": "ocaml.interface"}), nil, candidates(command("ocamllsp")), "Install ocaml-lsp-server and add ocamllsp to PATH."),
	server("perl", "Perl Navigator", languages("pl,pm,t", "perl"), nil, candidates(command("perlnavigator")), "Install Perl Navigator and add perlnavigator to PATH."),
	server("php", "Intelephense", languages("php,phtml", "php"), nil, candidates(commandArgs("intelephense", "--stdio")), "npm install -g intelephense"),
	server("prisma", "Prisma Language Server", languages("prisma", "prisma"), nil, candidates(commandArgs("prisma-language-server", "--stdio")), "npm install -g @prisma/language-server"),
	server("protobuf", "Protobuf Language Server", languages("proto", "proto"), nil, candidates(command("protols"), command("protobuf-language-server")), "Install protols or protobuf-language-server and add it to PATH."),
	server("python", "Python Language Server", languageMap(map[string]string{"py": "python", "pyi": "python"}), nil, candidates(commandArgs("basedpyright-langserver", "--stdio"), commandArgs("pyright-langserver", "--stdio"), command("pylsp")), "Install basedpyright, pyright, or python-lsp-server and add its launcher to PATH."),
	server("ruby", "Ruby Language Server", languageMap(map[string]string{"rb": "ruby", "rake": "ruby", "gemspec": "ruby"}), filenameMap(map[string]string{"gemfile": "ruby", "rakefile": "ruby"}), candidates(command("ruby-lsp"), commandArgs("solargraph", "stdio")), "Install ruby-lsp or solargraph and add it to PATH."),
	server("rust", "rust-analyzer", languages("rs", "rust"), nil, candidates(command("rust-analyzer")), "Install rust-analyzer and add it to PATH."),
	server("sql", "SQL Language Server", languages("sql", "sql"), nil, candidates(command("sqls")), "Install sqls and add it to PATH."),
	server("svelte", "Svelte Language Server", languages("svelte", "svelte"), nil, candidates(commandArgs("svelteserver", "--stdio")), "npm install -g svelte-language-server"),
	server("swift", "SourceKit-LSP", languages("swift", "swift"), nil, candidates(command("sourcekit-lsp")), "Install a Swift toolchain that includes sourcekit-lsp."),
	server("terraform", "Terraform Language Server", languages("tf,tfvars", "terraform"), nil, candidates(commandArgs("terraform-ls", "serve")), "Install terraform-ls and add it to PATH."),
	server("toml", "Taplo", languages("toml", "toml"), nil, candidates(commandArgs("taplo", "lsp", "stdio")), "Install Taplo and add it to PATH."),
	server("typescript", "TypeScript Language Server", languageMap(map[string]string{"js": "javascript", "jsx": "javascriptreact", "mjs": "javascript", "cjs": "javascript", "ts": "typescript", "tsx": "typescriptreact", "mts": "typescript", "cts": "typescript"}), nil, candidates(commandArgs("typescript-language-server", "--stdio")), "npm install -g typescript typescript-language-server"),
	server("v", "V Analyzer", languages("v,vsh", "v"), nil, candidates(command("v-analyzer")), "Install v-analyzer and add it to PATH."),
	server("vue", "Vue Language Server", languages("vue", "vue"), nil, candidates(commandArgs("vue-language-server", "--stdio")), "npm install -g @vue/language-server"),
	server("xml", "XML Language Server", languageMap(map[string]string{"xml": "xml", "xsd": "xml", "xsl": "xsl", "xslt": "xsl"}), nil, candidates(command("lemminx")), "Install LemMinX and add lemminx to PATH."),
	server("yaml", "YAML Language Server", languages("yaml,yml", "yaml"), nil, candidates(commandArgs("yaml-language-server", "--stdio")), "npm install -g yaml-language-server"),
	server("zig", "Zig Language Server", languages("zig,zon", "zig"), nil, candidates(command("zls")), "Install zls and add it to PATH."),
}

func server(id, name string, languageIDs, filenameIDs map[string]string, candidates []serverCandidate, installHint string) *serverDefinition {
	return &serverDefinition{id: id, name: name, languages: languageIDs, filenames: filenameIDs, candidates: candidates, installHint: installHint}
}

func languages(extensions, languageID string) map[string]string {
	result := make(map[string]string)
	for _, extension := range strings.Split(extensions, ",") {
		result[extension] = languageID
	}
	return result
}

func languageMap(values map[string]string) map[string]string { return values }
func filenameMap(values map[string]string) map[string]string { return values }
func command(name string) serverCandidate                    { return serverCandidate{command: name} }
func commandArgs(name string, args ...string) serverCandidate {
	return serverCandidate{command: name, args: args}
}
func candidates(values ...serverCandidate) []serverCandidate { return values }

func identifyServer(path string) (*serverDefinition, string) {
	filename := strings.ToLower(filepath.Base(strings.TrimSpace(path)))
	extension := strings.TrimPrefix(strings.ToLower(filepath.Ext(filename)), ".")
	if strings.HasPrefix(filename, "dockerfile.") || strings.HasPrefix(filename, "containerfile.") {
		return serverByID("docker"), "dockerfile"
	}
	if strings.HasSuffix(filename, ".tf.json") {
		return serverByID("terraform"), "terraform"
	}
	for _, definition := range serverRegistry {
		if languageID := definition.filenames[filename]; languageID != "" {
			return definition, languageID
		}
		if languageID := definition.languages[extension]; languageID != "" {
			return definition, languageID
		}
	}
	return nil, ""
}

func serverByID(id string) *serverDefinition {
	for _, definition := range serverRegistry {
		if definition.id == id {
			return definition
		}
	}
	return nil
}

func resolveServer(workspaceRoot, path string) (resolvedServer, bool) {
	definition, languageID := identifyServer(path)
	if definition == nil {
		return resolvedServer{}, false
	}
	for _, candidate := range definition.candidates {
		if binary := findServerBinary(workspaceRoot, candidate.command); binary != "" {
			return resolvedServer{definition: definition, candidate: candidate, binary: binary, languageID: languageID}, true
		}
	}
	return resolvedServer{definition: definition, languageID: languageID}, false
}

func findServerBinary(workspaceRoot, name string) string {
	for _, directory := range []string{
		filepath.Join(workspaceRoot, "node_modules", ".bin"),
		filepath.Join(workspaceRoot, ".venv", "bin"),
		filepath.Join(workspaceRoot, ".venv", "Scripts"),
		filepath.Join(workspaceRoot, "venv", "bin"),
		filepath.Join(workspaceRoot, "venv", "Scripts"),
	} {
		if binary, err := exec.LookPath(filepath.Join(directory, name)); err == nil {
			return binary
		}
	}
	if binary, err := exec.LookPath(name); err == nil {
		return binary
	}
	return ""
}
