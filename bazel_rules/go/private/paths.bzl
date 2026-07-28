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

def _validate_pattern_syntax(pattern):
    p_len = len(pattern)
    i = 0
    for _ in range(p_len):
        if i >= p_len:
            break
        c = pattern[i]
        if c == "\\":
            if i + 1 == p_len:
                fail("malformed pattern: trailing backslash in " + pattern)
            i += 2
        elif c == "[":
            i += 1
            start = i
            negated = False
            if i < p_len and pattern[i] == "^":
                negated = True
                i += 1
            close_idx = -1
            for _ in range(p_len):
                if i >= p_len:
                    break
                if pattern[i] == "\\":
                    if i + 1 == p_len:
                        fail("malformed pattern: trailing backslash in " + pattern)
                    i += 2
                elif pattern[i] == "]":
                    close_idx = i
                    break
                else:
                    i += 1
            if close_idx == -1:
                fail("malformed pattern: unclosed [ in " + pattern)
            class_body = pattern[start + (1 if negated else 0):close_idx]
            if class_body == "":
                fail("malformed pattern: empty character class in " + pattern)
            i = close_idx + 1
        else:
            i += 1

def _next_class_char(body, i, pattern):
    if body[i] == "\\":
        if i + 1 >= len(body):
            fail("malformed pattern: trailing backslash in " + pattern)
        return body[i + 1], i + 2
    return body[i], i + 1

def _match_char_class(char_class, ch, pattern):
    negated = False
    body = char_class
    if char_class.startswith("^"):
        negated = True
        body = char_class[1:]

    matched = False
    i = 0
    body_len = len(body)
    for _ in range(body_len):
        if i >= body_len:
            break
        c1, next_i = _next_class_char(body, i, pattern)
        if next_i < body_len and body[next_i] == "-" and next_i + 1 < body_len:
            c2, end_i = _next_class_char(body, next_i + 1, pattern)
            if c1 > c2:
                fail("malformed pattern: invalid range in " + pattern)
            if c1 <= ch and ch <= c2:
                matched = True
            i = end_i
        else:
            if ch == c1:
                matched = True
            i = next_i

    return not matched if negated else matched

def match_path(pattern, name):
    """Reports whether `name` matches `pattern` using Go path.Match semantics."""
    _validate_pattern_syntax(pattern)

    p_len = len(pattern)
    n_len = len(name)
    p = 0
    n = 0
    star_p = -1
    star_n = -1

    max_steps = (p_len + 1) * (n_len + 1) + 10
    for _ in range(max_steps):
        if n >= n_len:
            break

        if p < p_len and pattern[p] == "\\":
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
            i = p + 1
            for _ in range(p_len):
                if i >= p_len:
                    break
                if pattern[i] == "\\":
                    i += 2
                elif pattern[i] == "]":
                    close_idx = i
                    break
                else:
                    i += 1

            char_class = pattern[p + 1:close_idx]
            p = close_idx + 1

            if name[n] == "/":
                pass
            elif _match_char_class(char_class, name[n], pattern):
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

