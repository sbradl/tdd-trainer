package coach

import (
	"strings"
	"testing"

	"github.com/sbradl/tdd-trainer/internal/config"
	"github.com/sbradl/tdd-trainer/internal/snapshot"
)

// testDiff wraps one test file's unified diff.
func testDiff(path, patch string) []snapshot.FileDiff {
	return []snapshot.FileDiff{{Path: path, Kind: config.Test, Patch: patch}}
}

func TestNewTestCodeLeavesOutTidiedNeighbours(t *testing.T) {
	// a jest Red that also inlines a variable in the test before it
	file := `import {Add} from './calc'

describe('calc', () => {
    it('is 0 when empty', () => {
        expect(Add('')).toBe(0)
    })

    it('is 1 for 1', () => {
        expect(Add('1')).toBe(1)
    })
})
`
	patch := `--- a/calc.test.ts
+++ b/calc.test.ts
@@ -3,6 +3,9 @@
 describe('calc', () => {
     it('is 0 when empty', () => {
-        const result = Add('')
-        expect(result).toBe(0)
+        expect(Add('')).toBe(0)
+    })
+
+    it('is 1 for 1', () => {
+        expect(Add('1')).toBe(1)
     })
 })
`
	code, n := newTestCode(testDiff("calc.test.ts", patch), map[string]string{"calc.test.ts": file},
		[]string{"calc.test.ts::calc::is 1 for 1"})
	want := "    it('is 1 for 1', () => {\n        expect(Add('1')).toBe(1)\n    })"
	if code != want {
		t.Fatalf("code:\n%s\nwant:\n%s", code, want)
	}
	if n != 2 {
		t.Fatalf("lines = %d, want 2", n)
	}
}

func TestNewTestCodeKeepsDecoratorsAndCountsOnlyTheNewRow(t *testing.T) {
	file := `class TestCalc:
    @pytest.mark.parametrize('given, want',
                             [('1,2', 3),
                              ('4,5', 9)])
    def test_sum(self, given, want):
        assert add(given) == want

    def test_other(self):
        assert True
`
	patch := `--- a/test_calc.py
+++ b/test_calc.py
@@ -1,6 +1,7 @@
 class TestCalc:
     @pytest.mark.parametrize('given, want',
-                             [('1,2', 3)])
+                             [('1,2', 3),
+                              ('4,5', 9)])
     def test_sum(self, given, want):
         assert add(given) == want
`
	code, n := newTestCode(testDiff("test_calc.py", patch), map[string]string{"test_calc.py": file},
		[]string{"test_calc.TestCalc::test_sum[4,5-9]"})
	if !strings.HasPrefix(code, "    @pytest.mark.parametrize") || !strings.HasSuffix(code, "assert add(given) == want") {
		t.Fatalf("code:\n%s", code)
	}
	if n != 2 {
		t.Fatalf("lines = %d, want 2", n)
	}
}

func TestNewTestCodeFindsGoTestsAndBraceOnOwnLine(t *testing.T) {
	cases := []struct{ name, file, id, want string }{
		{"go subtest", "func TestA(t *testing.T) {\n\tt.Run(\"x\", nil)\n}\n\nfunc TestB(t *testing.T) {\n\tcheck(t, 2)\n}\n",
			"kata::TestB/two", "func TestB(t *testing.T) {\n\tcheck(t, 2)\n}"},
		{"go name prefix", "func TestBig(t *testing.T) {\n\tcheck(t, 9)\n}\n\nfunc TestB(t *testing.T) {\n\tcheck(t, 2)\n}\n",
			"kata::TestB", "func TestB(t *testing.T) {\n\tcheck(t, 2)\n}"},
		{"csharp", "    [Fact]\n    public void Adds()\n    {\n        Assert.Equal(2, Calc.Add(1, 1));\n    }\n\n    [Fact]\n    public void Other() { }\n",
			"Kata.Tests.CalcTests::Adds", "    [Fact]\n    public void Adds()\n    {\n        Assert.Equal(2, Calc.Add(1, 1));\n    }"},
		{"elixir describe", "  describe \"add\" do\n    test \"adds two numbers\" do\n      assert Kata.add(1, 2) == 3\n    end\n  end\n",
			"Elixir.KataTest::test add adds two numbers", "    test \"adds two numbers\" do\n      assert Kata.add(1, 2) == 3\n    end"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// every line is new
			var p strings.Builder
			p.WriteString("--- /dev/null\n+++ b/f\n@@ -0,0 +1 @@\n")
			for _, l := range strings.Split(strings.TrimSuffix(c.file, "\n"), "\n") {
				p.WriteString("+" + l + "\n")
			}
			code, _ := newTestCode(testDiff("f", p.String()), map[string]string{"f": c.file}, []string{c.id})
			if code != c.want {
				t.Fatalf("code:\n%s\nwant:\n%s", code, c.want)
			}
		})
	}
}

func TestNewTestCodeWithoutTheNameDropsReindentedLines(t *testing.T) {
	// a templated jest name cannot be found; the describe was re-indented
	file := "describe('a', () => {\n  describe('b', () => {\n    it('x', () => {\n      expect(f()).toBe(1)\n    })\n  })\n  it(`y ${n}`, () => {\n    expect(g()).toBe(2)\n  })\n})\n"
	patch := `--- a/f.test.js
+++ b/f.test.js
@@ -1,5 +1,10 @@
 describe('a', () => {
-  it('x', () => {
-    expect(f()).toBe(1)
-  })
+  describe('b', () => {
+    it('x', () => {
+      expect(f()).toBe(1)
+    })
+  })
+  it(` + "`y ${n}`" + `, () => {
+    expect(g()).toBe(2)
+  })
 })
`
	code, n := newTestCode(testDiff("f.test.js", patch), map[string]string{"f.test.js": file},
		[]string{"f.test.js::a::y 3"})
	if strings.Contains(code, "expect(f())") || !strings.Contains(code, "expect(g())") {
		t.Fatalf("code:\n%s", code)
	}
	if n != 5 {
		t.Fatalf("lines = %d, want 5", n)
	}
}
