// Package cli holds devvm's commands, made with Cobra, each in a file of
// its own named for it (root.go is devvm itself, which is up), and the
// hidden ones devvm runs itself, whose names start with _.
//
// A command says everything about itself where it's made: its Use, Short,
// Long and Example, its Args and flags, and, in its Annotations from an
// about, the docs page its help and CLI reference page link to, its
// section of the root's help and, for one without --json, why. NewRoot
// registers each, and help.go reads the abouts to group the root's help.
//
// The rest is shared by every command:
//
//   - app.go: the run's services, each made once (the folders, the test
//     overrides, the secrets, the backend registry, the shared tools, the
//     VM environment, the image builder), and a.env, which runs a command
//     on the folder's VM.
//   - args.go: the checks on arguments and flag values, whose failures are
//     usage errors; flags.go: the flags several commands share.
//   - word.go: devvm WORD and help's unknown words refused, a part's name
//     pointing to its flag and any other word getting a did-you-mean.
//   - help.go: the help's groups, Examples and docs links; json.go:
//     --json's output, its shapes checked in json_test.go.
//   - script_test.go runs the CLI tests; docgen/ writes the CLI reference.
package cli
