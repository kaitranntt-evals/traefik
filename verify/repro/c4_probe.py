#!/usr/bin/env python3
# Run: python3 verify/repro/c4_probe.py <claim-branch-worktree> <5a6681ac|6f1a5ee0|85523a87> — runs the branch's own added error-sanitization tests unmodified, then under production mutations that leak a confidential fragment (or drop the URN) from the returned error; every mutation is reverted with git checkout.
import subprocess, sys, os, re

wt, key = os.path.abspath(sys.argv[1]), sys.argv[2]
MW, AUTH, DYN = "pkg/server/middleware", "pkg/middlewares/auth", "pkg/config/dynamic"

# name -> list of (file, old, new); every `old` must occur exactly once.
MUT = {
 "5a6681ac": {
  "R  (redactor appends the raw construction/decoding error after the URN list)": [
    (MW+"/middlewares.go",
     'return fmt.Errorf("middleware construction failed with secret references: %s", strings.Join(refs, ", "))',
     'return fmt.Errorf("middleware construction failed with secret references: %s: %w", strings.Join(refs, ", "), err)')],
  "U  (redactor drops the URN list)": [
    (MW+"/middlewares.go",
     'return fmt.Errorf("middleware construction failed with secret references: %s", strings.Join(refs, ", "))',
     'return fmt.Errorf("middleware construction failed%s", strings.Repeat("", len(refs)))')],
  "V  (redactor keeps the URN list but appends only the resolved Secret value)": [
    (MW+"/middlewares.go",
     'return fmt.Errorf("middleware construction failed with secret references: %s", strings.Join(refs, ", "))',
     'return fmt.Errorf("middleware construction failed with secret references: %s: %s", strings.Join(refs, ", "), config.SecretValue(refs[0]))')],
  "A  (Secret-backed auth entry error carries the malformed entry through the redactor)": [
    (DYN+"/secrets.go",
     'return nil, &SecretUserError{Reference: entry, Index: i + 1}',
     'return nil, fmt.Errorf("%w: %s", &SecretUserError{Reference: entry, Index: i + 1}, user)'),
    (MW+"/middlewares.go", '\t\treturn userErr\n', '\t\treturn err\n')],
  "P  (auth parsers quote the malformed entry again, as on the base commit)": [
    (AUTH+"/basic_auth.go", 'fmt.Errorf("error parsing BasicUser")', 'fmt.Errorf("error parsing BasicUser: %v", user)'),
    (AUTH+"/digest_auth.go", 'fmt.Errorf("error parsing DigestUser")', 'fmt.Errorf("error parsing DigestUser: %v", user)')],
 },
 "6f1a5ee0": {
  "R  (redactor appends the raw construction/decoding error after the URN list)": [
    (MW+"/secrets.go",
     'return fmt.Errorf("cannot build middleware using secrets %s", refs)',
     'return fmt.Errorf("cannot build middleware using secrets %s: %w", refs, err)')],
  "U  (redactor drops the URN list)": [
    (MW+"/secrets.go",
     'return fmt.Errorf("cannot build middleware using secrets %s", refs)',
     'return fmt.Errorf("cannot build middleware using secrets%s", "")')],
  "V  (redactor keeps the URN list but appends only the resolved plugin configuration)": [
    (MW+"/secrets.go",
     'return fmt.Errorf("cannot build middleware using secrets %s", refs)',
     'return fmt.Errorf("cannot build middleware using secrets %s: %v", refs, config.Resolve().Plugin)')],
  "A  (auth parsers quote the malformed entry AND the redactor appends the raw auth error)": [
    (AUTH+"/basic_auth.go", 'errors.New("error parsing BasicUser")', 'errors.New("error parsing BasicUser: " + user)'),
    (AUTH+"/digest_auth.go", 'errors.New("error parsing DigestUser")', 'errors.New("error parsing DigestUser: " + user)'),
    (MW+"/secrets.go",
     'return fmt.Errorf("invalid user entry %d in %s", userErr.Index, refs)',
     'return fmt.Errorf("invalid user entry %d in %s: %w", userErr.Index, refs, err)')],
  "P  (auth parsers quote the malformed entry again, as on the base commit)": [
    (AUTH+"/basic_auth.go", 'errors.New("error parsing BasicUser")', 'errors.New("error parsing BasicUser: " + user)'),
    (AUTH+"/digest_auth.go", 'errors.New("error parsing DigestUser")', 'errors.New("error parsing DigestUser: " + user)')],
 },
 "85523a87": {
  "R  (plugin Build error: redacted error wraps the raw decoder error)": [
    (MW+"/middlewares.go",
     '\t\t\tif len(config.Secrets.References()) > 0 {\n\t\t\t\treturn nil, pluginSecretError(config.Secrets)\n\t\t\t}\n\t\t\treturn nil, fmt.Errorf("plugin: %w", err)',
     '\t\t\tif len(config.Secrets.References()) > 0 {\n\t\t\t\treturn nil, fmt.Errorf("%w: %w", pluginSecretError(config.Secrets), err)\n\t\t\t}\n\t\t\treturn nil, fmt.Errorf("plugin: %w", err)')],
  "U  (redactor drops the URN list)": [
    (MW+"/secrets.go",
     'return fmt.Errorf("plugin configuration or initialization failed with secret references %q", secrets.References())',
     'return fmt.Errorf("plugin configuration or initialization failed%s", "")')],
  "V  (plugin Build error: redacted error keeps the URN list but appends only the resolved Secret value)": [
    (MW+"/middlewares.go",
     '\t\t\tif len(config.Secrets.References()) > 0 {\n\t\t\t\treturn nil, pluginSecretError(config.Secrets)\n\t\t\t}\n\t\t\treturn nil, fmt.Errorf("plugin: %w", err)',
     '\t\t\tif len(config.Secrets.References()) > 0 {\n\t\t\t\treturn nil, fmt.Errorf("%w: %v", pluginSecretError(config.Secrets), rawPluginConfig["token"])\n\t\t\t}\n\t\t\treturn nil, fmt.Errorf("plugin: %w", err)')],
  "A  (Secret-backed auth entry error quotes the malformed entry)": [
    (MW+"/secrets.go",
     'return nil, fmt.Errorf("error parsing %s: entry %d from %q", kind, i+1, refs)',
     'return nil, fmt.Errorf("error parsing %s: entry %d from %q: %s", kind, i+1, refs, entry)')],
  "P  (getUsers quotes the malformed entry)": [
    (AUTH+"/auth.go",
     'return nil, fmt.Errorf("invalid user entry %d", i+1)',
     'return nil, fmt.Errorf("invalid user entry %d: %s", i+1, user)')],
 },
}[key]

def sh(cmd):
    return subprocess.run(cmd, shell=True, cwd=wt, capture_output=True, text=True)

added = sh("git diff --name-only --diff-filter=A af6d927c HEAD -- %s %s | grep _test.go" % (MW, AUTH)).stdout.split()
names = []
for f in added:
    names += re.findall(r"^func (Test\w+)\(", open(os.path.join(wt, f)).read(), re.M)
run = "^(%s)$" % "|".join(names)
cmd = "go test -count=1 -v -run '%s' ./%s ./%s" % (run, MW, AUTH)

def go(label):
    print("\n### " + label)
    print("$ " + cmd)
    out = sh(cmd)
    keep = re.compile(r"^\s*(--- (PASS|FAIL)|PASS|FAIL|ok\s)|Error Trace:|Error:\s|Messages:|should not contain|does not contain|\.go:\d+:")
    for line in (out.stdout + out.stderr).splitlines():
        if keep.search(line):
            print(line.rstrip()[:330])

print("### branch HEAD: " + sh("git rev-parse HEAD").stdout.strip())
print("### changed files before: " + str(len(sh("git status --short").stdout.splitlines())))
print("### added test files in scope: " + " ".join(added))
go("BASELINE (unmodified branch)")
for label, edits in MUT.items():
    files = sorted({f for f, _, _ in edits})
    try:
        for f, old, new in edits:
            p = os.path.join(wt, f); s = open(p).read()
            assert s.count(old) == 1, (f, old, s.count(old))
            open(p, "w").write(s.replace(old, new))
        print("\n### MUTATION " + label)
        print(sh("git diff -U0 -- " + " ".join(files) + " | grep -E '^[-+][^-+]'").stdout.rstrip())
        go("run under MUTATION " + label.split()[0])
    finally:
        sh("git checkout -- " + " ".join(files))
print("\n### changed files after: " + str(len(sh("git status --short").stdout.splitlines())))
