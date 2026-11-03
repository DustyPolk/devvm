package msg

// AnswerFor is what answers the question whose message has this ID, so
// that devvm needn't ask it: a flag, or a command run first. With
// --no-input, devvm names it instead of asking (no_input). "" is a
// question asked only in a terminal, which no flag answers, and an ID
// that isn't a question. TestAnswers checks every ID is a message.
func AnswerFor(id string) string {
	switch id {
	case "net_ask":
		return "--network-policy or --no-network-policy"
	case "use_parent_vm", "use_parent_vm_repo":
		return "--here, or devvm in the parent folder"
	case "size_cpus_prompt", "size_memory_prompt":
		return "devvm size -g"
	case "features_help":
		return "devvm setup --features"
	case "agents_help":
		return "devvm setup --agents"
	case "layers_help":
		return "devvm setup --layers"
	case "pick_new_help":
		return "--tools, --agents and --layers"
	case "pick_vm_help":
		return "devvm tools add or devvm tools rm"
	case "network_default_help":
		return "devvm network -g"
	case "load_key":
		return "devvm ssh-agent add KEY_FILE"
	case "restart_now":
		return "devvm restart"
	case "add_it_now":
		return "--ssh-config"
	case "remake_unfinished":
		return "devvm rm, then devvm"
	case "token_prompt":
		return "--stdin"
	case "continue", "gc_confirm", "gc_cache_confirm", "adopt_vm", "rm_confirm", "images_rm_confirm", "secret_rm_confirm":
		return "--force"
	}
	return ""
}
