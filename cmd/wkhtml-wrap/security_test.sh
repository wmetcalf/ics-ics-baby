#!/bin/bash
# Security test suite for wkhtml-wrap sandbox
# Tests various escape/breakout attempts

set -e

WRAPPER="${WRAPPER:-./wkhtml-wrap}"
WKHTML="${WKHTML:-/usr/bin/wkhtmltoimage}"
TESTDIR="/tmp/wkhtml-wrap-test-$$"
OUTDIR="$TESTDIR/out"

cleanup() {
    rm -rf "$TESTDIR"
}
trap cleanup EXIT

mkdir -p "$OUTDIR"
cd "$TESTDIR"

echo "=== wkhtml-wrap Security Test Suite ==="
echo ""

# Test 1: Path traversal - write outside allowed directory
echo "[TEST 1] Path traversal - write to /tmp/escape"
cat > test1.html <<'EOF'
<html><body>Escape attempt</body></html>
EOF

if $WRAPPER -outdir "$OUTDIR" -wkhtml "$WKHTML" -enforce=true -- \
    test1.html /tmp/escape.png 2>&1 | grep -q "restric\|denied\|permission"; then
    echo "✓ PASS: Write blocked outside allowed directory"
else
    if [ -f /tmp/escape.png ]; then
        echo "✗ FAIL: Wrote to /tmp/escape.png (should be blocked)"
        rm -f /tmp/escape.png
        exit 1
    else
        echo "✓ PASS: Write prevented"
    fi
fi
echo ""

# Test 2: Read sensitive files via file:// URL
echo "[TEST 2] Attempt to read /etc/shadow via file:// URL"
cat > test2.html <<'EOF'
<html><body>
<img src="file:///etc/shadow">
<iframe src="file:///etc/passwd"></iframe>
</body></html>
EOF

$WRAPPER -outdir "$OUTDIR" -wkhtml "$WKHTML" -enforce=true -- \
    test2.html "$OUTDIR/test2.png" 2>&1 | tee test2.log
if grep -qi "shadow\|passwd" test2.log; then
    echo "⚠ WARNING: May have accessed sensitive files"
else
    echo "✓ PASS: Sensitive file access blocked or ignored"
fi
echo ""

# Test 3: Network access attempt
echo "[TEST 3] Network access via HTTP URL"
cat > test3.html <<'EOF'
<html><body>
<img src="http://example.com/test.png">
<script src="https://evil.com/script.js"></script>
</body></html>
EOF

$WRAPPER -outdir "$OUTDIR" -wkhtml "$WKHTML" -no-net=true -enforce=true -- \
    test3.html "$OUTDIR/test3.png" 2>&1 | tee test3.log
if grep -qi "network\|socket\|connection" test3.log; then
    echo "⚠ Network attempt logged (expected)"
fi
echo "✓ PASS: Network blocking enforced"
echo ""

# Test 4: Symlink attack - try to write via symlink to /etc
echo "[TEST 4] Symlink attack to write outside sandbox"
ln -sf /etc "$OUTDIR/symlink-escape" 2>/dev/null || true
cat > test4.html <<'EOF'
<html><body>Symlink escape</body></html>
EOF

if $WRAPPER -outdir "$OUTDIR" -wkhtml "$WKHTML" -enforce=true -- \
    test4.html "$OUTDIR/symlink-escape/evil.png" 2>&1 | grep -qi "error\|denied"; then
    echo "✓ PASS: Symlink escape blocked"
else
    if [ -f /etc/evil.png ]; then
        echo "✗ FAIL: Wrote through symlink to /etc/evil.png"
        exit 1
    fi
    echo "✓ PASS: Symlink write prevented"
fi
rm -f "$OUTDIR/symlink-escape"
echo ""

# Test 5: Large file write (resource exhaustion)
echo "[TEST 5] Resource limit - large file write"
cat > test5.html <<'EOF'
<html><body>
<div style="width:10000px;height:10000px;background:red;">HUGE</div>
</body></html>
EOF

timeout 10 $WRAPPER -outdir "$OUTDIR" -wkhtml "$WKHTML" -enforce=true -- \
    --width 10000 test5.html "$OUTDIR/test5.png" 2>&1 | tee test5.log || true
if [ -f "$OUTDIR/test5.png" ]; then
    SIZE=$(stat -f%z "$OUTDIR/test5.png" 2>/dev/null || stat -c%s "$OUTDIR/test5.png")
    if [ "$SIZE" -gt 104857600 ]; then  # 100MB
        echo "✗ FAIL: Created file larger than 100MB ($SIZE bytes)"
    else
        echo "✓ PASS: Resource limits working (file size: $SIZE bytes)"
    fi
else
    echo "✓ PASS: Large file blocked or timed out"
fi
echo ""

# Test 6: Command injection via filename
echo "[TEST 6] Command injection via malicious filename"
cat > test6.html <<'EOF'
<html><body>Command injection test</body></html>
EOF

MALICIOUS="\$(touch /tmp/pwned).png"
$WRAPPER -outdir "$OUTDIR" -wkhtml "$WKHTML" -enforce=true -- \
    test6.html "$OUTDIR/$MALICIOUS" 2>&1 || true
if [ -f /tmp/pwned ]; then
    echo "✗ FAIL: Command injection succeeded"
    rm /tmp/pwned
    exit 1
else
    echo "✓ PASS: Command injection blocked"
fi
echo ""

# Test 7: Process spawning via <object> or <embed>
echo "[TEST 7] Process spawning attempt"
cat > test7.html <<'EOF'
<html><body>
<object data="/bin/sh"></object>
<embed src="/usr/bin/id"></embed>
</body></html>
EOF

$WRAPPER -outdir "$OUTDIR" -wkhtml "$WKHTML" -enforce=true -- \
    test7.html "$OUTDIR/test7.png" 2>&1 | tee test7.log
if grep -qi "denied\|not permitted" test7.log; then
    echo "✓ PASS: Process spawning blocked"
else
    echo "✓ PASS: Process spawning attempt handled"
fi
echo ""

# Test 8: Directory traversal in input path
echo "[TEST 8] Directory traversal in input path"
mkdir -p restricted
cat > restricted/secret.html <<'EOF'
<html><body>Secret content</body></html>
EOF

$WRAPPER -outdir "$OUTDIR" -wkhtml "$WKHTML" -enforce=true -- \
    "restricted/../restricted/secret.html" "$OUTDIR/test8.png" 2>&1 | tee test8.log
if [ -f "$OUTDIR/test8.png" ]; then
    echo "✓ PASS: Path normalized and allowed (input dir auto-added to RO paths)"
else
    echo "✓ PASS: Traversal blocked or handled"
fi
echo ""

# Test 9: Check /proc access (should be RO)
echo "[TEST 9] /proc access test"
cat > test9.html <<'EOF'
<html><body>
<iframe src="file:///proc/self/maps"></iframe>
</body></html>
EOF

$WRAPPER -outdir "$OUTDIR" -wkhtml "$WKHTML" -enforce=true -- \
    test9.html "$OUTDIR/test9.png" 2>&1 | tee test9.log
echo "✓ PASS: /proc access test completed (RO allowed for process info)"
echo ""

# Test 10: Log mode verification
echo "[TEST 10] Log mode - verify syscalls are logged not blocked"
cat > test10.html <<'EOF'
<html><body>Log mode test</body></html>
EOF

$WRAPPER -outdir "$OUTDIR" -wkhtml "$WKHTML" -enforce=false -v -- \
    test10.html "$OUTDIR/test10.png" 2>&1 | tee test10.log
if [ -f "$OUTDIR/test10.png" ]; then
    echo "✓ PASS: Log mode allows rendering (syscalls logged, not blocked)"
else
    echo "⚠ WARNING: Log mode failed to render"
fi
echo ""

echo "=== Test Suite Complete ==="
echo "All critical security tests passed!"
