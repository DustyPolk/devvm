package msg

// docs links messages to the docs page that says more: message ID to
// "page#anchor", a page in docs/content/docs and a heading on it. devvm
// prints the page's address on the line after the message's text.
// TestDocLinks checks every entry.
var docs = map[string]string{
	"backend_lacks":           "backends#missing-and-unreviewed-features",
	"backend_unreviewed":      "backends#missing-and-unreviewed-features",
	"backend_unknown":         "backends#choosing-a-backend",
	"backend_unknown_default": "backends#choosing-a-backend",
	"backend_unknown_vm":      "backends#choosing-a-backend",
	"lima_missing":            "lima#requirements",
	"lima_broken":             "lima#requirements",
	"lima_old":                "lima#requirements",
	"lima_host_unsupported":   "lima#requirements",
	"qemu_missing":            "lima#requirements",
	"lima_global_config":      "lima#a-lima-home-of-its-own",
	"vm_has_networks":         "lima#a-lima-home-of-its-own",
	"lima_home_too_long":      "lima#limits",
	"mount_comma":             "lima#limits",
	"disk_shrink":             "lima#limits",
	"image_in_use":            "lima#clones-and-overlays",
	"qemu_refused":            "lima#confinement",
	"start_no_ssh":            "lima#limits",
	"no_ubuntu_image":         "images#ubuntus-cloud-image",
	"ubuntu_bad_signature":    "images#ubuntus-cloud-image",
	"ubuntu_unsigned":         "images#ubuntus-cloud-image",
	"unknown_layer":           "images#layers",
	"script_failed":           "images#build-logs-and-interrupts",
	"rebuild_replaces":        "images#rebuilding",
	"vm_outdated":             "images#outdated-vms",
}
