# iele

These are the pieces she keeps.

In Romanian folklore the iele are spirits — sprites or fairies with roots in
the Dacian lands. They appear at night, dance in circles, and are spoken of
carefully because their attention is not always welcome. The name felt fitting.

This is not a framework. It is a small collection of consistent tools for the
parts of a program that are usually tedious to write from scratch. The goal is
to make the boring work less scattered so that building on top of it can be
quicker and more reliable.

The main.go that gets stamped is not intended to be a finished program. It
exists mainly as a clear example of how the pieces are meant to fit together.
What comes after is left to whatever uses them.

## The pieces

These are always included:
- arg: command-line argument parsing
- err: categorized error handling
- pipe: input and output through pipes or files

These can be added when wanted:
- cfg: configuration file parsing and binding
- proc: running external processes, capturing output, and detached background launch
- sec: secure token loading with permission checks
- tmp: temporary file and directory handling with signal watching
- turn: message recording and projection using the write-ahead log (pulls in wal)
- wal: append-only write-ahead logging
- web: HTTP client with multipart support

## To use

```sh
./iele -n project-name -a "Your Name" [-l mit] [-p cfg,web,...] [-h] [-v]
```

It creates a new directory with the chosen pieces copied in (imports adjusted),
a license, go.mod, and the example main.go.

`-h` shows help, `-v` prints the version.

The stamped result uses the internal packages directly. It is a starting point,
not a complete application.

## License

Copyright (c) Tomoe (warawatomoe@proton.me)

SPDX-License-Identifier: MIT OR Apache-2.0 OR BSD-2-Clause

This project is licensed under any of the three licenses at your option.

- [LICENSE-MIT](LICENSE-MIT)
- [LICENSE-APACHE-2.0](LICENSE-APACHE-2.0)
- [LICENSE-BSD-2-Clause](LICENSE-BSD-2-Clause)
