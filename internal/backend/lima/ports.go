package lima

// Forwarding ports (ports, ports-all) is emulated, and reviewed: Lima's
// own forwards are off, as every VM's portForwards ignore every guest port
// on every address (noForwardsSet, set where they differ), so Lima
// forwards nothing (its ssh is the VM driver's forward, not a rule).
// devvm's forwarder, which isn't the backend's (vmssh.Forwards), forwards
// the VM's ports instead, over Lima's ssh as SSH gives it (its host
// loopback port, Lima's user and Lima's own key, _config/user in devvm's
// Lima home, which every VM there authorizes), and ends once the VM's
// hostagent does (its lifeline, ha.pid). Which ports a VM forwards is
// never in lima.yaml.
