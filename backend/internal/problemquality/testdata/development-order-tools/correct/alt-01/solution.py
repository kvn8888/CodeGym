def normalize_tags(tags):
    seen = set()
    ordered = []
    for tag in tags:
        normalized = str(tag).strip().lower()
        if normalized not in seen:
            seen.add(normalized)
            ordered.append(normalized)
    return ordered
