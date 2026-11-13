package asset

import "testing"

func TestRoleOf(t *testing.T) {
	cases := map[string]Role{
		"data/image/ev/001.png":        RoleCG,
		"data/evimage/cg001.png":       RoleCG,
		"data/image/bg/001.png":        RoleBG,
		"data/bgimage/black.png":       RoleBG,
		"black":                        RoleBG,
		"data/image/fg/chara_a.png":    RoleStand,
		"data/fgimage/tami/z1/a.png":   RoleStand,
		"data/image/sys/menu.png":      RoleUI,
		"title_logo.png":               RoleUI,
		"data/movie/op.mpg":            RoleMovie,
		"video/ed01.webm":              RoleMovie,
		"data/image/mystery_asset.png": RoleMisc,
	}
	for path, want := range cases {
		if got := RoleOf(path); got != want {
			t.Errorf("RoleOf(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestRelativeKeepsSubdirs(t *testing.T) {
	cases := map[string]string{
		"data/image/ev/a.png":     "ev/a.png",
		"data/image/bg/a.png":     "bg/a.png",
		"raw/data/fgimage/tami/a": "tami/a",
		"data/movie/op.mpg":       "op.mpg",
		"../../etc/passwd.png":    "etc/passwd.png",
		`C:\data\image\a.png`:     "a.png",
	}
	for in, want := range cases {
		if got := Relative(in); got != want {
			t.Errorf("Relative(%q) = %q, want %q", in, got, want)
		}
	}
}
