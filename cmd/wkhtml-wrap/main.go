//go:build linux

// wkhtml-wrap: sandboxed runner for wkhtmltoimage with optional "log mode".
//
// Usage:
//   wkhtml-wrap -outdir /out [-wkhtml /usr/bin/wkhtmltoimage] \
//     [-ro /usr/share/fonts -ro /opt/assets] [-no-net=true] \
//     [-block-clone3=true] [-no-exec-mem=true] [-enforce=true] [-v] -- \
//     input.html /out/output.png
//
// Defaults added by wrapper:
//   --enable-local-file-access             (always)
//   --disable-javascript                   (unless you pass --enable-javascript)
//
// Security (when -enforce=true):
// - Writes allowed ONLY to /tmp and -outdir (Landlock; needs Linux >= 5.13).
// - Network disabled (AF_INET/AF_INET6).
// - New *processes* blocked (threads allowed).
// - Namespaces/bpf/ptrace/mount/kexec/module loading/etc. denied.
// - Optional "no exec-mem" (JIT blocker) when JS is disabled.
//
// Log Mode (when -enforce=false):
// - All deny rules switch to seccomp ActLog (syscalls are ALLOWED but logged).
// - Use this for a dry run to see hits before enforcing.

package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	seccomp "github.com/seccomp/libseccomp-golang"
	"golang.org/x/sys/unix"

	"github.com/landlock-lsm/go-landlock/landlock"
)

var (
	outDir      string
	wkhtmlPath  string
	roPaths     multiFlag
	noNet       bool
	blockClone3 bool
	noExecMem   bool
	enforce     bool
	verbose     bool
)

type multiFlag []string

func (m *multiFlag) String() string     { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error { *m = append(*m, v); return nil }

func vprintf(format string, a ...any) {
	if verbose {
		log.Printf(format, a...)
	}
}

func main() {
	log.SetFlags(0)
	flag.StringVar(&outDir, "outdir", "", "output directory to allow RW")
	flag.StringVar(&wkhtmlPath, "wkhtml", "wkhtmltoimage", "path to wkhtmltoimage binary")
	flag.Var(&roPaths, "ro", "additional read-only dir (repeatable)")
	flag.BoolVar(&noNet, "no-net", true, "deny AF_INET/AF_INET6 sockets")
	flag.BoolVar(&blockClone3, "block-clone3", false, "deny clone3 (enable to block, but may break Qt/glibc thread creation)")
	flag.BoolVar(&noExecMem, "no-exec-mem", false, "forbid anonymous executable memory (enable to block, but may break Qt5 WebKit)")
	flag.BoolVar(&enforce, "enforce", true, "enforce seccomp denies (true) or just log (false)")
	flag.BoolVar(&verbose, "v", false, "verbose logs")
	flag.Parse()

	passArgs := flag.Args()
	if outDir == "" {
		log.Fatal("missing -outdir (required)")
	}

	// Resolve wkhtmltoimage path.
	if p, err := execPath(wkhtmlPath); err == nil {
		wkhtmlPath = p
	} else {
		log.Fatalf("wkhtmltoimage not found: %v", err)
	}

	// Create isolated temp directory for this execution
	tmpDir, err := os.MkdirTemp("/tmp", "wkhtml-wrap-")
	if err != nil {
		log.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir) // Clean up after exec (won't run since we exec, but good practice)

	// Build Landlock allowlists.
	roAllow := map[string]struct{}{}
	rwAllow := map[string]struct{}{tmpDir: {}, outDir: {}}

	// Baseline RO dirs typically needed by glibc/Qt/fontconfig/etc.
	for _, p := range []string{
		"/usr", "/lib", "/lib64", "/bin", "/sbin", "/etc",
		"/dev", "/proc", "/sys", "/run", "/var",
	} {
		roAllow[p] = struct{}{}
	}
	// User-specified RO dirs.
	for _, p := range roPaths {
		roAllow[filepath.Clean(p)] = struct{}{}
	}
	// Auto-add RO parents for existing file/dir args (inputs, assets).
	for _, a := range passArgs {
		if a == "-" || strings.HasPrefix(a, "-") || strings.Contains(a, "://") {
			continue // flag/stdin/URL (URL would be blocked by no net anyway)
		}
		clean := filepath.Clean(a)
		if fi, err := os.Stat(clean); err == nil {
			if fi.IsDir() {
				roAllow[clean] = struct{}{}
			} else {
				roAllow[filepath.Dir(clean)] = struct{}{}
			}
		}
	}

	// --- Argument hardening / defaults ---
	// Always allow local file access, and disable JS by default unless explicitly enabled.
	passArgs = ensureFront(passArgs, "--enable-local-file-access",
		[]string{"--enable-local-file-access", "--disable-local-file-access"})
	jsDisabled := !contains(passArgs, "--enable-javascript")
	if jsDisabled {
		passArgs = ensureFront(passArgs, "--disable-javascript",
			[]string{"--disable-javascript", "-n"}) // avoid dup
	}

	// Apply sandbox on this thread before exec.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	// Required for unprivileged seccomp/landlock.
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		log.Fatalf("PR_SET_NO_NEW_PRIVS failed: %v", err)
	}
	// Hygiene: disable core dumps.
	_ = unix.Prctl(unix.PR_SET_DUMPABLE, 0, 0, 0, 0)

	// Restrictive umask: owner-only permissions (0077)
	_ = unix.Umask(0077)

	// 1) Landlock filesystem policy.
	roList := keys(roAllow)
	rwList := keys(rwAllow)
	vprintf("Landlock RO: %v", roList)
	vprintf("Landlock RW: %v", rwList)
	// Use V5 for network rules + ioctl restrictions (falls back to V4/V3/V2/V1 gracefully)
	if err := landlock.V5.BestEffort().RestrictPaths(
		landlock.RODirs(roList...),
		landlock.RWDirs(rwList...),
	); err != nil {
		log.Fatalf("landlock RestrictPaths failed: %v", err)
	}

	// 2) seccomp policy.
	filter, err := seccomp.NewFilter(seccomp.ActAllow)
	if err != nil {
		log.Fatalf("seccomp filter create: %v", err)
	}
	deny := denyAction(enforce)

	// Networking off (keep UNIX sockets).
	if noNet {
		cond4, _ := seccomp.MakeCondition(0, seccomp.CompareEqual, uint64(unix.AF_INET))
		cond6, _ := seccomp.MakeCondition(0, seccomp.CompareEqual, uint64(unix.AF_INET6))
		must(filter.AddRuleConditional(seccomp.ScmpSyscall(unix.SYS_SOCKET), deny, []seccomp.ScmpCondition{cond4}))
		must(filter.AddRuleConditional(seccomp.ScmpSyscall(unix.SYS_SOCKET), deny, []seccomp.ScmpCondition{cond6}))
		if sc, err := seccomp.GetSyscallFromName("socketcall"); err == nil && sc != seccomp.ScmpSyscall(-1) {
			must(filter.AddRule(sc, deny))
		}
	}

	// Disallow new *processes*; allow threads.
	must(filter.AddRule(seccomp.ScmpSyscall(unix.SYS_FORK), deny))
	must(filter.AddRule(seccomp.ScmpSyscall(unix.SYS_VFORK), deny))
	const CLONE_THREAD = 0x00010000
	condNoThread, _ := seccomp.MakeCondition(0, seccomp.CompareMaskedEqual, uint64(CLONE_THREAD), 0)
	must(filter.AddRuleConditional(seccomp.ScmpSyscall(unix.SYS_CLONE), deny, []seccomp.ScmpCondition{condNoThread}))
	if blockClone3 {
		if sc, err := seccomp.GetSyscallFromName("clone3"); err == nil && sc != seccomp.ScmpSyscall(-1) {
			must(filter.AddRule(sc, deny))
		}
	}

	// Namespaces & other risky syscalls.
	denyList := []string{
		"unshare", "setns",
		"ptrace",
		"bpf", "perf_event_open",
		"userfaultfd",
		"add_key", "request_key", "keyctl",
		"mount", "umount2", "pivot_root", "chroot",
		"kexec_load", "kexec_file_load",
		"init_module", "finit_module",
		"reboot",
		"open_by_handle_at", "name_to_handle_at",
		"io_uring_setup",
		// Additional dangerous syscalls
		"acct", "lookup_dcookie", "kcmp",
		"process_vm_readv", "process_vm_writev",
		"swapon", "swapoff",
		"vhangup",
		"settimeofday", "clock_settime", "adjtimex",
		"delete_module",
		"quotactl",
		"nfsservctl",
		"_sysctl",
	}
	for _, name := range denyList {
		if sc, err := seccomp.GetSyscallFromName(name); err == nil && sc != seccomp.ScmpSyscall(-1) {
			must(filter.AddRule(sc, deny))
		}
	}

	// If JS is disabled (default) and no-exec-mem is on, harden against JIT.
	if jsDisabled && noExecMem {
		hardenNoExecMem(filter, deny)
	}

	if err := filter.Load(); err != nil {
		log.Fatalf("seccomp Load failed: %v", err)
	}

	// Resource caps (best-effort).
	_ = unix.Setrlimit(unix.RLIMIT_NOFILE, &unix.Rlimit{Cur: 512, Max: 512})
	_ = unix.Setrlimit(unix.RLIMIT_FSIZE, &unix.Rlimit{Cur: 100 << 20, Max: 100 << 20}) // 100 MiB
	_ = unix.Setrlimit(unix.RLIMIT_CPU, &unix.Rlimit{Cur: 60, Max: 60})                   // 60 seconds CPU time
	_ = unix.Setrlimit(unix.RLIMIT_AS, &unix.Rlimit{Cur: 2 << 30, Max: 2 << 30})         // 2 GiB address space

	// Keep caches inside isolated temp directory (not global /tmp for security).
	env := append(os.Environ(),
		"HOME="+tmpDir,
		"XDG_CACHE_HOME="+filepath.Join(tmpDir, "xdg-cache"),
		"TMPDIR="+tmpDir,
	)

	vprintf("enforce=%v; exec: %s -- %v", enforce, wkhtmlPath, passArgs)
	if err := syscall.Exec(wkhtmlPath, append([]string{wkhtmlPath}, passArgs...), env); err != nil {
		log.Fatalf("exec failed: %v", err)
	}
}

func execPath(p string) (string, error) {
	if strings.ContainsRune(p, os.PathSeparator) {
		if st, err := os.Stat(p); err == nil && !st.IsDir() && st.Mode()&0111 != 0 {
			return p, nil
		}
		return "", fmt.Errorf("not executable: %s", p)
	}
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		full := filepath.Join(dir, p)
		if st, err := os.Stat(full); err == nil && !st.IsDir() && st.Mode()&0111 != 0 {
			return full, nil
		}
	}
	return "", errors.New("not found in PATH")
}

func keys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func contains(args []string, needle string) bool {
	for _, a := range args {
		if a == needle {
			return true
		}
	}
	return false
}

// ensureFront adds flag 'want' to the front of args unless any of 'seen' are present.
func ensureFront(args []string, want string, seen []string) []string {
	for _, s := range seen {
		if contains(args, s) {
			return args
		}
	}
	return append([]string{want}, args...)
}

// Choose deny action: enforce => EPERM; log-mode => LOG (allows + logs).
func denyAction(enforce bool) seccomp.ScmpAction {
	if enforce {
		return seccomp.ActErrno.SetReturnCode(int16(unix.EPERM))
	}
	return seccomp.ActLog
}

// Forbid anonymous executable memory (JIT / self-modifying code) while allowing
// file-backed executable mappings for shared libraries and the binary itself.
func hardenNoExecMem(f *seccomp.ScmpFilter, deny seccomp.ScmpAction) {
	const PROT_EXEC = 0x4
	const MAP_ANONYMOUS = 0x20

	// mprotect(addr, len, prot): deny when adding PROT_EXEC.
	if sc, err := seccomp.GetSyscallFromName("mprotect"); err == nil && sc != seccomp.ScmpSyscall(-1) {
		cond, _ := seccomp.MakeCondition(2, seccomp.CompareMaskedEqual, uint64(PROT_EXEC), uint64(PROT_EXEC)) // prot
		must(f.AddRuleConditional(sc, deny, []seccomp.ScmpCondition{cond}))
	}

	// mmap(addr, length, prot, flags, fd, offset): deny when ANON and PROT_EXEC.
	addNoAnonExecRule := func(name string) {
		if sc, err := seccomp.GetSyscallFromName(name); err == nil && sc != seccomp.ScmpSyscall(-1) {
			cProt, _ := seccomp.MakeCondition(2, seccomp.CompareMaskedEqual, uint64(PROT_EXEC), uint64(PROT_EXEC))      // prot
			cAnon, _ := seccomp.MakeCondition(3, seccomp.CompareMaskedEqual, uint64(MAP_ANONYMOUS), uint64(MAP_ANONYMOUS)) // flags
			must(f.AddRuleConditional(sc, deny, []seccomp.ScmpCondition{cProt, cAnon}))
		}
	}
	addNoAnonExecRule("mmap")
	addNoAnonExecRule("mmap2") // 32-bit compat
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
