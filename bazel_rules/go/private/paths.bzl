"""The runfiles-root-relative path frame shared across the Go rules.

Every path arcc reads — interface files, layout sources, the manifest and
layout given on the command line — is expressed relative to the runfiles root,
because the check runs with the runfiles root as its working directory. It is
the one frame in which a main-repo source and an external-repo source can both
be named without `..` segments, which arcc rejects as escaping the workspace.

The `_go_component` rule writes paths in this frame; the `arcc_check_test`
launcher `cd`s to the runfiles root so those paths resolve. They must therefore
agree on how the frame is computed, which is why this lives in one place.
"""

def runfiles_path(ctx, file):
    """Path of `file` relative to the runfiles root."""
    short_path = file.short_path
    if short_path.startswith("../"):
        return short_path[len("../"):]
    return ctx.workspace_name + "/" + short_path

def _match_char_class(char_class, ch):
    if not char_class:
        return False

    i = 0
    negated = False
    if char_class[0] == "^":
        negated = True
        i = 1

    matched = False
    class_len = len(char_class)
    for _ in range(class_len):
        if i >= class_len:
            break
        if i + 2 < class_len and char_class[i + 1] == "-":
            low = char_class[i]
            high = char_class[i + 2]
            if low <= ch and ch <= high:
                matched = True
                break
            i += 3
        else:
            if char_class[i] == ch:
                matched = True
                break
            i += 1

    return not matched if negated else matched

def match_path(pattern, name):
    """Reports whether `name` matches `pattern` using Go path.Match semantics."""
    p_len = len(pattern)
    n_len = len(name)
    p = 0
    n = 0
    star_p = -1
    star_n = -1

    max_steps = (p_len + 1) * (n_len + 1) + 1
    for _ in range(max_steps):
        if n >= n_len:
            break

        if p < p_len and pattern[p] == "\\":
            if p + 1 == p_len:
                fail("malformed pattern: trailing backslash in " + pattern)
            p += 1
            if pattern[p] == name[n]:
                p += 1
                n += 1
                continue
        elif p < p_len and pattern[p] == "?":
            if name[n] != "/":
                p += 1
                n += 1
                continue
        elif p < p_len and pattern[p] == "[":
            close_idx = -1
            for idx in range(p + 1, p_len):
                if pattern[idx] == "]":
                    close_idx = idx
                    break
            if close_idx == -1:
                fail("malformed pattern: unclosed [ in " + pattern)

            char_class = pattern[p + 1:close_idx]
            p = close_idx + 1

            if name[n] == "/":
                pass
            elif _match_char_class(char_class, name[n]):
                n += 1
                continue
        elif p < p_len and pattern[p] == "*":
            star_p = p
            star_n = n
            p += 1
            continue
        elif p < p_len and pattern[p] == name[n]:
            p += 1
            n += 1
            continue

        if star_p != -1 and name[star_n] != "/":
            star_n += 1
            n = star_n
            p = star_p + 1
            continue

        return False

    for _ in range(p_len):
        if p < p_len and pattern[p] == "*":
            p += 1

    return p == p_len and n == n_len

