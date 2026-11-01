package msg

import "testing"

// Every question AnswerFor answers is a message, and an ID that isn't a
// question has no answer.
func TestAnswers(t *testing.T) {
	questions := []string{
		"net_ask", "use_parent_vm", "use_parent_vm_repo", "size_cpus_prompt", "size_memory_prompt",
		"features_help", "backend_help", "network_default_help", "load_key", "update_mounts", "add_it_now",
		"token_prompt", "continue", "gc_confirm", "gc_cache_confirm", "adopt_vm", "rm_confirm",
		"images_rm_confirm", "secret_rm_confirm",
	}
	for _, id := range questions {
		if _, ok := specs[id]; !ok {
			t.Errorf("%s: no such message in locales/en.toml", id)
		}
		if AnswerFor(id) == "" {
			t.Errorf("%s: no answer", id)
		}
	}
	for _, id := range []string{"", "no_vm", "delete_parent_vm", "delete_anyway", "widen_parent_vm"} {
		if a := AnswerFor(id); a != "" {
			t.Errorf("AnswerFor(%q) = %q, want none", id, a)
		}
	}
}
