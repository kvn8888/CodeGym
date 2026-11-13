def normalize_tags(tags):
    return sorted({str(tag).strip() for tag in tags})
