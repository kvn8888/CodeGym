#!/usr/bin/env python3
"""Manage the CodeGym GitHub Projects v2 kanban board.

This script intentionally avoids the `gh` CLI so agents can use it in Codex
environments where GitHub's project-column tools are not exposed. It needs a
token with project scope in GH_TOKEN or GITHUB_TOKEN.
"""

from __future__ import annotations

import argparse
import json
import os
import ssl
import sys
import textwrap
import time
import urllib.error
import urllib.parse
import urllib.request
from dataclasses import dataclass
from typing import Any

API_URL = "https://api.github.com/graphql"
DEFAULT_OWNER = "kvn8888"
DEFAULT_REPO = "CodeGym"
DEFAULT_PROJECT_TITLE = "CodeGym"
PROJECT_FIELD_ARGS = ("status", "category", "priority", "size", "source")
PROJECT_FIELD_NAMES = {
    "status": "Status",
    "category": "Category",
    "priority": "Priority",
    "size": "Size",
    "source": "Source",
}


def default_ssl_context() -> ssl.SSLContext | None:
    """Use certifi when the host Python lacks a usable macOS CA store."""
    try:
        import certifi  # type: ignore
    except ImportError:
        return None
    return ssl.create_default_context(cafile=certifi.where())


SSL_CONTEXT = default_ssl_context()


class GitHubError(RuntimeError):
    pass


@dataclass(frozen=True)
class ProjectRef:
    id: str
    number: int
    title: str
    url: str
    fields: list[dict[str, Any]]


class GitHubClient:
    def __init__(self, token: str) -> None:
        self.token = token

    def rest(self, method: str, path: str, payload: dict[str, Any] | None = None) -> dict[str, Any]:
        body = json.dumps(payload).encode() if payload is not None else None
        req = urllib.request.Request(
            f"https://api.github.com{path}",
            data=body,
            headers={
                "Authorization": f"Bearer {self.token}",
                "Content-Type": "application/json",
                "Accept": "application/vnd.github+json",
                "User-Agent": "codegym-project-board-script",
            },
            method=method,
        )
        try:
            with urllib.request.urlopen(req, timeout=30, context=SSL_CONTEXT) as res:
                raw = res.read().decode()
        except urllib.error.HTTPError as err:
            detail = err.read().decode(errors="replace")
            raise GitHubError(f"GitHub REST API HTTP {err.code}: {detail}") from err
        except urllib.error.URLError as err:
            raise GitHubError(f"GitHub REST API request failed: {err}") from err

        return json.loads(raw) if raw else {}

    def graphql(self, query: str, variables: dict[str, Any] | None = None) -> dict[str, Any]:
        body = json.dumps({"query": query, "variables": variables or {}}).encode()
        req = urllib.request.Request(
            API_URL,
            data=body,
            headers={
                "Authorization": f"Bearer {self.token}",
                "Content-Type": "application/json",
                "Accept": "application/vnd.github+json",
                "User-Agent": "codegym-project-board-script",
            },
            method="POST",
        )
        try:
            with urllib.request.urlopen(req, timeout=30, context=SSL_CONTEXT) as res:
                payload = json.loads(res.read().decode())
        except urllib.error.HTTPError as err:
            detail = err.read().decode(errors="replace")
            raise GitHubError(f"GitHub API HTTP {err.code}: {detail}") from err
        except urllib.error.URLError as err:
            raise GitHubError(f"GitHub API request failed: {err}") from err

        if payload.get("errors"):
            messages = "; ".join(error.get("message", str(error)) for error in payload["errors"])
            raise GitHubError(messages)
        return payload["data"]


USER_PROJECTS_QUERY = """
query UserProjects($login: String!, $first: Int!) {
  user(login: $login) {
    projectsV2(first: $first, orderBy: {field: UPDATED_AT, direction: DESC}) {
      nodes {
        id
        number
        title
        url
        fields(first: 50) {
          nodes {
            __typename
            ... on ProjectV2FieldCommon {
              id
              name
              dataType
            }
            ... on ProjectV2SingleSelectField {
              id
              name
              dataType
              options { id name }
            }
          }
        }
      }
    }
  }
}
"""

ORG_PROJECTS_QUERY = """
query OrgProjects($login: String!, $first: Int!) {
  organization(login: $login) {
    projectsV2(first: $first, orderBy: {field: UPDATED_AT, direction: DESC}) {
      nodes {
        id
        number
        title
        url
        fields(first: 50) {
          nodes {
            __typename
            ... on ProjectV2FieldCommon {
              id
              name
              dataType
            }
            ... on ProjectV2SingleSelectField {
              id
              name
              dataType
              options { id name }
            }
          }
        }
      }
    }
  }
}
"""

PROJECT_ITEMS_QUERY = """
query ProjectItems($projectId: ID!, $first: Int!, $after: String) {
  node(id: $projectId) {
    ... on ProjectV2 {
      items(first: $first, after: $after) {
        pageInfo { hasNextPage endCursor }
        nodes {
          id
          type
          content {
            __typename
            ... on Issue {
              id
              number
              title
              url
              state
              body
              repository { nameWithOwner }
              assignees(first: 10) { nodes { login } }
              labels(first: 10) { nodes { name } }
            }
            ... on PullRequest {
              id
              number
              title
              url
              state
              body
              repository { nameWithOwner }
              assignees(first: 10) { nodes { login } }
              labels(first: 10) { nodes { name } }
            }
            ... on DraftIssue {
              id
              title
              body
            }
          }
          fieldValues(first: 50) {
            nodes {
              __typename
              ... on ProjectV2ItemFieldTextValue {
                text
                field { ... on ProjectV2FieldCommon { id name dataType } }
              }
              ... on ProjectV2ItemFieldSingleSelectValue {
                name
                optionId
                field { ... on ProjectV2FieldCommon { id name dataType } }
              }
              ... on ProjectV2ItemFieldDateValue {
                date
                field { ... on ProjectV2FieldCommon { id name dataType } }
              }
              ... on ProjectV2ItemFieldNumberValue {
                number
                field { ... on ProjectV2FieldCommon { id name dataType } }
              }
              ... on ProjectV2ItemFieldUserValue {
                users(first: 10) { nodes { login } }
                field { ... on ProjectV2FieldCommon { id name dataType } }
              }
              ... on ProjectV2ItemFieldLabelValue {
                labels(first: 10) { nodes { name } }
                field { ... on ProjectV2FieldCommon { id name dataType } }
              }
              ... on ProjectV2ItemFieldMilestoneValue {
                milestone { title }
                field { ... on ProjectV2FieldCommon { id name dataType } }
              }
              ... on ProjectV2ItemFieldRepositoryValue {
                repository { nameWithOwner }
                field { ... on ProjectV2FieldCommon { id name dataType } }
              }
            }
          }
        }
      }
    }
  }
}
"""

REPOSITORY_ISSUE_QUERY = """
query RepositoryIssue($owner: String!, $repo: String!, $number: Int!) {
  repository(owner: $owner, name: $repo) {
    issue(number: $number) {
      id
      number
      title
      url
    }
  }
}
"""

ADD_ITEM_MUTATION = """
mutation AddItem($projectId: ID!, $contentId: ID!) {
  addProjectV2ItemById(input: {projectId: $projectId, contentId: $contentId}) {
    item { id }
  }
}
"""

ADD_DRAFT_MUTATION = """
mutation AddDraft($projectId: ID!, $title: String!, $body: String) {
  addProjectV2DraftIssue(input: {projectId: $projectId, title: $title, body: $body}) {
    projectItem { id }
  }
}
"""

DELETE_ITEM_MUTATION = """
mutation DeleteItem($projectId: ID!, $itemId: ID!) {
  deleteProjectV2Item(input: {projectId: $projectId, itemId: $itemId}) {
    deletedItemId
  }
}
"""

UPDATE_FIELD_MUTATION = """
mutation UpdateField($projectId: ID!, $itemId: ID!, $fieldId: ID!, $value: ProjectV2FieldValue!) {
  updateProjectV2ItemFieldValue(input: {
    projectId: $projectId,
    itemId: $itemId,
    fieldId: $fieldId,
    value: $value
  }) {
    projectV2Item { id }
  }
}
"""

UPDATE_DRAFT_MUTATION = """
mutation UpdateDraft($draftIssueId: ID!, $title: String, $body: String) {
  updateProjectV2DraftIssue(input: {
    draftIssueId: $draftIssueId,
    title: $title,
    body: $body
  }) {
    draftIssue { id title body }
  }
}
"""

UPDATE_ISSUE_MUTATION = """
mutation UpdateIssue($issueId: ID!, $title: String, $body: String) {
  updateIssue(input: {
    id: $issueId,
    title: $title,
    body: $body
  }) {
    issue { id number title url }
  }
}
"""


def token_from_env() -> str:
    token = os.environ.get("GH_TOKEN") or os.environ.get("GITHUB_TOKEN")
    if not token:
        raise GitHubError("Set GH_TOKEN or GITHUB_TOKEN with repo/project permissions.")
    return token


def list_projects(client: GitHubClient, owner: str) -> list[ProjectRef]:
    lookup_errors: list[str] = []
    for query, owner_key in ((USER_PROJECTS_QUERY, "user"), (ORG_PROJECTS_QUERY, "organization")):
        try:
            data = client.graphql(query, {"login": owner, "first": 50})
        except GitHubError as err:
            lookup_errors.append(str(err))
            continue

        owner_data = data.get(owner_key)
        if owner_data:
            return project_refs(owner_data["projectsV2"]["nodes"])

    if lookup_errors:
        raise GitHubError("; ".join(lookup_errors))
    raise GitHubError(f"Could not find GitHub user or organization {owner!r}.")


def project_refs(nodes: list[dict[str, Any]]) -> list[ProjectRef]:
    return [
        ProjectRef(
            id=node["id"],
            number=node["number"],
            title=node["title"],
            url=node["url"],
            fields=[field for field in node["fields"]["nodes"] if field],
        )
        for node in nodes
    ]


def resolve_project(args: argparse.Namespace, client: GitHubClient) -> ProjectRef:
    projects = list_projects(client, args.owner)
    if args.project_number is not None:
        for project in projects:
            if project.number == args.project_number:
                return project
        raise GitHubError(f"No project #{args.project_number} found for {args.owner}.")

    target_title = args.project_title.lower()
    for project in projects:
        if project.title.lower() == target_title:
            return project
    for project in projects:
        if target_title in project.title.lower():
            return project

    project_list = ", ".join(f"#{project.number} {project.title}" for project in projects)
    raise GitHubError(
        f"No project matching title {args.project_title!r}. "
        f"Pass --project-number. Available: {project_list or 'none'}"
    )


def status_field(project: ProjectRef) -> dict[str, Any]:
    for field in project.fields:
        if field.get("name", "").lower() == "status" and field.get("options"):
            return field
    raise GitHubError(f"Project {project.title!r} does not have a single-select Status field.")


def field_by_name(project: ProjectRef, name: str) -> dict[str, Any]:
    for field in project.fields:
        if field.get("name", "").lower() == name.lower():
            return field
    names = ", ".join(field.get("name", "") for field in project.fields)
    raise GitHubError(f"No project field named {name!r}. Available fields: {names}")


def option_by_name(field: dict[str, Any], name: str) -> dict[str, Any]:
    options = field.get("options") or []
    for option in options:
        if option["name"].lower() == name.lower():
            return option
    for option in options:
        if name.lower() in option["name"].lower():
            return option
    names = ", ".join(option["name"] for option in options)
    raise GitHubError(f"No option {name!r} for field {field['name']!r}. Available: {names}")


def list_items(client: GitHubClient, project_id: str, page_size: int = 100) -> list[dict[str, Any]]:
    items: list[dict[str, Any]] = []
    after = None
    while True:
        data = client.graphql(
            PROJECT_ITEMS_QUERY,
            {"projectId": project_id, "first": page_size, "after": after},
        )
        node = data["node"]
        if not node:
            raise GitHubError("Project node was not found.")
        page = node["items"]
        items.extend(page["nodes"])
        if not page["pageInfo"]["hasNextPage"]:
            return items
        after = page["pageInfo"]["endCursor"]


def item_field_values(item: dict[str, Any]) -> dict[str, str]:
    values: dict[str, str] = {}
    for value in item["fieldValues"]["nodes"]:
        field = value.get("field") or {}
        name = field.get("name")
        if not name:
            continue
        typename = value["__typename"]
        if typename == "ProjectV2ItemFieldSingleSelectValue":
            values[name] = value.get("name") or ""
        elif typename == "ProjectV2ItemFieldTextValue":
            values[name] = value.get("text") or ""
        elif typename == "ProjectV2ItemFieldDateValue":
            values[name] = value.get("date") or ""
        elif typename == "ProjectV2ItemFieldNumberValue":
            values[name] = str(value.get("number") or "")
        elif typename == "ProjectV2ItemFieldUserValue":
            values[name] = ", ".join(user["login"] for user in value["users"]["nodes"])
        elif typename == "ProjectV2ItemFieldLabelValue":
            values[name] = ", ".join(label["name"] for label in value["labels"]["nodes"])
        elif typename == "ProjectV2ItemFieldMilestoneValue":
            milestone = value.get("milestone")
            values[name] = milestone["title"] if milestone else ""
        elif typename == "ProjectV2ItemFieldRepositoryValue":
            repo = value.get("repository")
            values[name] = repo["nameWithOwner"] if repo else ""
    return values


def item_title(item: dict[str, Any]) -> str:
    content = item.get("content") or {}
    return content.get("title") or "(untitled)"


def item_number(item: dict[str, Any]) -> str:
    content = item.get("content") or {}
    number = content.get("number")
    return f"#{number}" if number else "draft"


def item_url(item: dict[str, Any]) -> str:
    content = item.get("content") or {}
    return content.get("url") or ""


def item_content_id(item: dict[str, Any]) -> str:
    content = item.get("content") or {}
    return content.get("id") or ""


def item_issue_number_value(item: dict[str, Any]) -> int | None:
    number = (item.get("content") or {}).get("number")
    return int(number) if number is not None else None


def find_repo_issue_by_title(client: GitHubClient, args: argparse.Namespace, title: str) -> dict[str, Any] | None:
    """Find an exact-title issue even when it is not already on the Project board."""
    query = f'repo:{repo_slug(args)} is:issue in:title "{title}"'
    payload = client.rest("GET", f"/search/issues?q={urllib.parse.quote(query)}&per_page=20")
    matches = [
        item for item in payload.get("items", [])
        if item.get("title") == title and "pull_request" not in item
    ]
    if not matches:
        return None

    matches.sort(key=lambda item: (item.get("state") != "open", item.get("number", 0)))
    return matches[0]


def markdown_escape(text: str) -> str:
    return text.replace("|", "\\|").replace("\n", " ")


def print_projects(projects: list[ProjectRef]) -> None:
    print("| Number | Title | URL |")
    print("| --- | --- | --- |")
    for project in projects:
        print(f"| {project.number} | {markdown_escape(project.title)} | {project.url} |")


def print_columns(project: ProjectRef) -> None:
    field = status_field(project)
    print(f"# {project.title} Status Columns")
    print()
    for option in field["options"]:
        print(f"- {option['name']} (`{option['id']}`)")


def print_fields(project: ProjectRef) -> None:
    print(f"# {project.title} Fields")
    print()
    for field in project.fields:
        print(f"- {field.get('name')} (`{field.get('dataType')}`)")
        for option in field.get("options") or []:
            print(f"  - {option['name']} (`{option['id']}`)")


def print_items_markdown(items: list[dict[str, Any]], group: bool = True) -> None:
    if group:
        grouped: dict[str, list[dict[str, Any]]] = {}
        for item in items:
            status = item_field_values(item).get("Status", "No Status")
            grouped.setdefault(status, []).append(item)
        for status, status_items in grouped.items():
            print(f"## {status}")
            print()
            print("| Item | Title | Type | Assignees | Labels | URL |")
            print("| --- | --- | --- | --- | --- | --- |")
            for item in status_items:
                print(item_row(item))
            print()
        return

    print("| Status | Item | Title | Type | Assignees | Labels | URL |")
    print("| --- | --- | --- | --- | --- | --- | --- |")
    for item in items:
        status = item_field_values(item).get("Status", "No Status")
        print(f"| {markdown_escape(status)} | {item_row(item)}")


def item_row(item: dict[str, Any]) -> str:
    content = item.get("content") or {}
    assignees = ", ".join(user["login"] for user in content.get("assignees", {}).get("nodes", []))
    labels = ", ".join(label["name"] for label in content.get("labels", {}).get("nodes", []))
    return (
        f"| {item_number(item)} "
        f"| {markdown_escape(item_title(item))} "
        f"| {content.get('__typename', item.get('type', ''))} "
        f"| {markdown_escape(assignees)} "
        f"| {markdown_escape(labels)} "
        f"| {item_url(item)} |"
    )


def print_item_detail(item: dict[str, Any]) -> None:
    content = item.get("content") or {}
    values = item_field_values(item)
    print(f"# {item_number(item)} {item_title(item)}")
    print()
    print(f"- Project item ID: `{item['id']}`")
    if item_content_id(item):
        print(f"- Content ID: `{item_content_id(item)}`")
    print(f"- Type: `{content.get('__typename', item.get('type', ''))}`")
    if item_url(item):
        print(f"- URL: {item_url(item)}")
    for name, value in values.items():
        print(f"- {name}: {value}")
    body = content.get("body")
    if body:
        print()
        print("## Body")
        print()
        print(body)


def resolve_item(items: list[dict[str, Any]], ref: str) -> dict[str, Any]:
    clean = ref.strip()
    if clean.startswith("PVTI_"):
        for item in items:
            if item["id"] == clean:
                return item

    if clean.startswith("#"):
        clean = clean[1:]
    if clean.isdigit():
        for item in items:
            if str((item.get("content") or {}).get("number", "")) == clean:
                return item

    for item in items:
        content = item.get("content") or {}
        if clean in {item["id"], content.get("id", ""), content.get("url", "")}:
            return item
        if clean.lower() == item_title(item).lower():
            return item

    raise GitHubError(f"No project item matched {ref!r}. Use list to find the item ID.")


def find_item_by_id(items: list[dict[str, Any]], item_id: str) -> dict[str, Any] | None:
    for item in items:
        if item["id"] == item_id:
            return item
    return None


def find_item_by_title(items: list[dict[str, Any]], title: str) -> dict[str, Any] | None:
    normalized = title.strip().lower()
    for item in items:
        if item_title(item).strip().lower() == normalized:
            return item
    return None


def issue_content_id(client: GitHubClient, owner: str, repo: str, number: int) -> str:
    data = client.graphql(REPOSITORY_ISSUE_QUERY, {"owner": owner, "repo": repo, "number": number})
    issue = data["repository"]["issue"] if data.get("repository") else None
    if not issue:
        raise GitHubError(f"No issue #{number} found in {owner}/{repo}.")
    return issue["id"]


def repo_slug(args: argparse.Namespace) -> str:
    return f"{args.repo_owner}/{args.repo}"


def issue_url(args: argparse.Namespace, number: int) -> str:
    return f"https://github.com/{repo_slug(args)}/issues/{number}"


def issue_number_from_url(url: str) -> int:
    try:
        return int(url.rstrip("/").split("/")[-1])
    except ValueError as err:
        raise GitHubError(f"Could not parse issue number from {url!r}.") from err


def split_labels(labels: list[str] | None) -> list[str]:
    if not labels:
        return []
    values: list[str] = []
    for label in labels:
        values.extend(part.strip() for part in label.split(",") if part.strip())
    return values


def labels_from_value(value: Any) -> list[str]:
    if value is None:
        return []
    if isinstance(value, str):
        return split_labels([value])
    if isinstance(value, list):
        labels: list[str] = []
        for item in value:
            if not isinstance(item, str):
                raise GitHubError(f"Labels must be strings, got {item!r}.")
            labels.extend(split_labels([item]))
        return labels
    raise GitHubError(f"Labels must be a comma-separated string or list of strings, got {value!r}.")


def add_labels_to_issue(client: GitHubClient, args: argparse.Namespace, number: int, labels: list[str]) -> None:
    if labels:
        client.rest("POST", f"/repos/{repo_slug(args)}/issues/{number}/labels", {"labels": labels})


def project_item_for_issue(client: GitHubClient, project: ProjectRef, number: int) -> dict[str, Any] | None:
    for item in list_items(client, project.id):
        content = item.get("content") or {}
        if str(content.get("number", "")) == str(number):
            return item
    return None


def ensure_issue_on_project(args: argparse.Namespace, client: GitHubClient, project: ProjectRef, number: int) -> str:
    existing = project_item_for_issue(client, project, number)
    if existing:
        return existing["id"]

    content_id = issue_content_id(client, args.repo_owner, args.repo, number)
    data = client.graphql(ADD_ITEM_MUTATION, {"projectId": project.id, "contentId": content_id})
    return data["addProjectV2ItemById"]["item"]["id"]


def set_project_field(client: GitHubClient, project: ProjectRef, item_id: str, field_name: str, value: str) -> None:
    field = field_by_name(project, field_name)
    client.graphql(
        UPDATE_FIELD_MUTATION,
        {
            "projectId": project.id,
            "itemId": item_id,
            "fieldId": field["id"],
            "value": mutation_value(field, value),
        },
    )


def project_field_values_from_args(args: argparse.Namespace) -> dict[str, str]:
    field_values: dict[str, str] = {}
    for attr in PROJECT_FIELD_ARGS:
        value = getattr(args, attr, None)
        if value:
            field_values[PROJECT_FIELD_NAMES[attr]] = value
    return field_values


def has_project_field_args(args: argparse.Namespace) -> bool:
    return bool(project_field_values_from_args(args))


def apply_project_fields(
    args: argparse.Namespace,
    client: GitHubClient,
    project: ProjectRef,
    item_id: str,
) -> dict[str, str]:
    field_values = project_field_values_from_args(args)
    for field_name, value in field_values.items():
        set_project_field(client, project, item_id, field_name, value)
    return field_values


def verify_project_fields(
    client: GitHubClient,
    project: ProjectRef,
    item_id: str,
    expected: dict[str, str],
    attempts: int = 8,
) -> None:
    if not expected:
        return

    missing: dict[str, tuple[str, str]] = {}
    for attempt in range(1, attempts + 1):
        item = find_item_by_id(list_items(client, project.id), item_id)
        if not item:
            if attempt < attempts:
                time.sleep(1)
                continue
            raise GitHubError(f"Project item {item_id!r} was not visible during verification.")

        values = item_field_values(item)
        missing = {
            field_name: (expected_value, values.get(field_name, ""))
            for field_name, expected_value in expected.items()
            if values.get(field_name, "") != expected_value
        }
        if not missing:
            print(f"Verified project fields for `{item_id}`.")
            return
        if attempt < attempts:
            time.sleep(1)

    details = ", ".join(
        f"{field}: expected {expected!r}, saw {actual!r}"
        for field, (expected, actual) in missing.items()
    )
    raise GitHubError(f"Project field readback mismatch for {item_id}: {details}")


def apply_and_maybe_verify_project_fields(
    args: argparse.Namespace,
    client: GitHubClient,
    project: ProjectRef,
    item_id: str,
) -> None:
    expected = apply_project_fields(args, client, project, item_id)
    if getattr(args, "verify", False):
        verify_project_fields(client, project, item_id, expected)


def mutation_value(field: dict[str, Any], value: str) -> dict[str, Any]:
    data_type = field.get("dataType")
    if data_type == "SINGLE_SELECT":
        option = option_by_name(field, value)
        return {"singleSelectOptionId": option["id"]}
    if data_type == "DATE":
        return {"date": value}
    if data_type == "NUMBER":
        return {"number": float(value)}
    return {"text": value}


def cmd_projects(args: argparse.Namespace, client: GitHubClient) -> None:
    print_projects(list_projects(client, args.owner))


def cmd_columns(args: argparse.Namespace, client: GitHubClient) -> None:
    print_columns(resolve_project(args, client))


def cmd_fields(args: argparse.Namespace, client: GitHubClient) -> None:
    print_fields(resolve_project(args, client))


def cmd_list(args: argparse.Namespace, client: GitHubClient) -> None:
    project = resolve_project(args, client)
    items = list_items(client, project.id)
    if args.status:
        items = [
            item
            for item in items
            if item_field_values(item).get("Status", "").lower() == args.status.lower()
        ]
    print_items_markdown(items, group=not args.flat)


def cmd_show(args: argparse.Namespace, client: GitHubClient) -> None:
    project = resolve_project(args, client)
    item = resolve_item(list_items(client, project.id), args.item)
    print_item_detail(item)


def cmd_add_issue(args: argparse.Namespace, client: GitHubClient) -> None:
    project = resolve_project(args, client)
    item_id = ensure_issue_on_project(args, client, project, args.number)
    apply_and_maybe_verify_project_fields(args, client, project, item_id)
    print(f"Added/updated issue #{args.number} on {project.title}: `{item_id}`")


def cmd_add_draft(args: argparse.Namespace, client: GitHubClient) -> None:
    project = resolve_project(args, client)
    body = read_body_arg(args.body, args.body_file)
    data = client.graphql(ADD_DRAFT_MUTATION, {"projectId": project.id, "title": args.title, "body": body})
    item_id = data["addProjectV2DraftIssue"]["projectItem"]["id"]
    print(f"Added draft to {project.title}: `{item_id}`")
    apply_and_maybe_verify_project_fields(args, client, project, item_id)


def cmd_delete(args: argparse.Namespace, client: GitHubClient) -> None:
    project = resolve_project(args, client)
    item = resolve_item(list_items(client, project.id), args.item)
    client.graphql(DELETE_ITEM_MUTATION, {"projectId": project.id, "itemId": item["id"]})
    print(f"Deleted project item `{item['id']}`.")


def cmd_move(args: argparse.Namespace, client: GitHubClient) -> None:
    project = resolve_project(args, client)
    item = resolve_item(list_items(client, project.id), args.item)
    set_status(client, project, item["id"], args.status)
    print(f"Moved `{item['id']}` to {args.status}.")


def set_status(client: GitHubClient, project: ProjectRef, item_id: str, status: str) -> None:
    field = status_field(project)
    option = option_by_name(field, status)
    client.graphql(
        UPDATE_FIELD_MUTATION,
        {
            "projectId": project.id,
            "itemId": item_id,
            "fieldId": field["id"],
            "value": {"singleSelectOptionId": option["id"]},
        },
    )


def cmd_set_field(args: argparse.Namespace, client: GitHubClient) -> None:
    project = resolve_project(args, client)
    item = resolve_item(list_items(client, project.id), args.item)
    set_project_field(client, project, item["id"], args.field, args.value)
    print(f"Set {args.field} on `{item['id']}` to {args.value!r}.")


def cmd_set_fields(args: argparse.Namespace, client: GitHubClient) -> None:
    project = resolve_project(args, client)
    item = resolve_item(list_items(client, project.id), args.item)
    apply_and_maybe_verify_project_fields(args, client, project, item["id"])
    print(f"Updated project item: `{item['id']}`")


def cmd_create_issue(args: argparse.Namespace, client: GitHubClient) -> None:
    payload: dict[str, Any] = {
        "title": args.title,
        "body": read_body_arg(args.body, args.body_file) or "",
    }
    labels = split_labels(args.label)
    if labels:
        payload["labels"] = labels

    issue = client.rest("POST", f"/repos/{repo_slug(args)}/issues", payload)
    number = int(issue["number"])
    print(f"Created issue #{number}: {issue.get('html_url', issue_url(args, number))}")

    if not args.no_project:
        project = resolve_project(args, client)
        item_id = ensure_issue_on_project(args, client, project, number)
        apply_and_maybe_verify_project_fields(args, client, project, item_id)
        print(f"Added/updated project item: `{item_id}`")


def cmd_edit_issue(args: argparse.Namespace, client: GitHubClient) -> None:
    payload: dict[str, Any] = {}
    body = read_body_arg(args.body, args.body_file)
    if args.title is not None:
        payload["title"] = args.title
    if body is not None:
        payload["body"] = body
    if args.state is not None:
        payload["state"] = args.state
    if payload:
        client.rest("PATCH", f"/repos/{repo_slug(args)}/issues/{args.number}", payload)

    add_labels_to_issue(client, args, args.number, split_labels(args.add_label))

    print(f"Edited issue #{args.number}: {issue_url(args, args.number)}")

    if has_project_field_args(args):
        project = resolve_project(args, client)
        item_id = ensure_issue_on_project(args, client, project, args.number)
        apply_and_maybe_verify_project_fields(args, client, project, item_id)
        print(f"Updated project item: `{item_id}`")


def upsert_issue_by_title(
    args: argparse.Namespace,
    client: GitHubClient,
    *,
    title: str,
    body: str | None,
    labels: list[str],
    state: str | None,
    close: bool,
    dry_run: bool = False,
) -> tuple[str, int | None, str | None]:
    project = resolve_project(args, client)
    existing = find_item_by_title(list_items(client, project.id), title)
    issue_number: int | None = None
    item_id: str | None = None
    action = "create"

    if existing:
        content = existing.get("content") or {}
        typename = content.get("__typename")
        if typename != "Issue":
            raise GitHubError(
                f"Existing project item titled {title!r} is a {typename}; "
                "delete/rename it or use a GitHub Issue before upserting."
            )
        issue_number = item_issue_number_value(existing)
        if issue_number is None:
            raise GitHubError(f"Existing issue titled {title!r} has no issue number.")
        item_id = existing["id"]
        action = "update"
    else:
        repo_issue = find_repo_issue_by_title(client, args, title)
        if repo_issue:
            issue_number = int(repo_issue["number"])
            action = "update"

    target_state = "closed" if close else state
    field_args = argparse.Namespace(**vars(args))
    if close and not getattr(field_args, "status", None):
        field_args.status = "Done"

    if dry_run:
        print(f"Would {action} issue: {title}")
        return action, issue_number, item_id

    if issue_number is None:
        payload: dict[str, Any] = {"title": title, "body": body or ""}
        if labels:
            payload["labels"] = labels
        issue = client.rest("POST", f"/repos/{repo_slug(args)}/issues", payload)
        issue_number = int(issue["number"])
        print(f"Created issue #{issue_number}: {issue.get('html_url', issue_url(args, issue_number))}")
    else:
        payload = {}
        if body is not None:
            payload["body"] = body
        if target_state is not None:
            payload["state"] = target_state
        if payload:
            client.rest("PATCH", f"/repos/{repo_slug(args)}/issues/{issue_number}", payload)
        add_labels_to_issue(client, args, issue_number, labels)
        print(f"Updated issue #{issue_number}: {issue_url(args, issue_number)}")

    if target_state is not None and action == "create":
        client.rest("PATCH", f"/repos/{repo_slug(args)}/issues/{issue_number}", {"state": target_state})

    item_id = ensure_issue_on_project(args, client, project, issue_number)
    apply_and_maybe_verify_project_fields(field_args, client, project, item_id)
    print(f"Added/updated project item: `{item_id}`")
    return action, issue_number, item_id


def cmd_upsert_issue(args: argparse.Namespace, client: GitHubClient) -> None:
    body = read_body_arg(args.body, args.body_file)
    action, number, _ = upsert_issue_by_title(
        args,
        client,
        title=args.title,
        body=body,
        labels=split_labels(args.label),
        state=args.state,
        close=args.close,
        dry_run=args.dry_run,
    )
    if args.dry_run:
        print("Dry run complete; no GitHub issue or project fields were changed.")
        return
    if number is not None:
        print(f"{action.title()}d issue #{number}.")


def cmd_import_issues(args: argparse.Namespace, client: GitHubClient) -> None:
    sources = [bool(args.file), bool(args.json), bool(args.stdin)]
    if sum(sources) > 1:
        raise GitHubError("Provide only one of --file, --json, or --stdin.")

    if args.json:
        raw = args.json
        base_dir = os.getcwd()
    elif args.stdin or args.file in (None, "-"):
        raw = sys.stdin.read()
        base_dir = os.getcwd()
    else:
        with open(args.file, "r", encoding="utf-8") as handle:
            raw = handle.read()
        base_dir = os.path.dirname(os.path.abspath(args.file))

    if not raw.strip():
        raise GitHubError(
            "No issue JSON provided. Pass --json '<json>', pipe JSON via stdin, or use --file <path>."
        )

    try:
        payload = json.loads(raw)
    except json.JSONDecodeError as err:
        raise GitHubError(f"Could not parse issue JSON: {err}") from err

    entries = payload.get("issues") if isinstance(payload, dict) else payload
    if not isinstance(entries, list):
        raise GitHubError("Import JSON must be a JSON list or an object with an `issues` list.")
    created: list[str] = []
    updated: list[str] = []
    closed: list[str] = []

    for index, entry in enumerate(entries, start=1):
        if not isinstance(entry, dict):
            raise GitHubError(f"Import entry {index} must be an object.")
        title = entry.get("title")
        if not isinstance(title, str) or not title.strip():
            raise GitHubError(f"Import entry {index} is missing a non-empty title.")

        body = entry.get("body")
        body_file = entry.get("bodyFile") or entry.get("body_file")
        if body_file:
            body_path = body_file if os.path.isabs(body_file) else os.path.join(base_dir, body_file)
            body = read_body_arg(None, body_path)
        elif body is not None and not isinstance(body, str):
            raise GitHubError(f"Import entry {index} body must be a string.")

        entry_args = argparse.Namespace(**vars(args))
        entry_args.status = entry.get("status", args.status)
        entry_args.category = entry.get("category", args.category)
        entry_args.priority = entry.get("priority", args.priority)
        entry_args.size = entry.get("size", args.size)
        entry_args.source = entry.get("source", args.source)
        entry_args.verify = args.verify

        close = bool(entry.get("close", False))
        state = entry.get("state")
        if state not in {None, "open", "closed"}:
            raise GitHubError(f"Import entry {index} has invalid state {state!r}.")
        labels = labels_from_value(entry.get("labels", args.label))

        action, number, _ = upsert_issue_by_title(
            entry_args,
            client,
            title=title,
            body=body,
            labels=labels,
            state=state,
            close=close,
            dry_run=args.dry_run,
        )
        label = f"#{number} {title}" if number is not None else title
        if close or state == "closed":
            closed.append(label)
        elif action == "create":
            created.append(label)
        else:
            updated.append(label)

    print("CREATED")
    for item in created:
        print(item)
    print("UPDATED")
    for item in updated:
        print(item)
    print("CLOSED")
    for item in closed:
        print(item)


def cmd_complete(args: argparse.Namespace, client: GitHubClient) -> None:
    project = resolve_project(args, client)
    item = resolve_item(list_items(client, project.id), args.item)
    content = item.get("content") or {}
    number = content.get("number")
    if number:
        client.rest("PATCH", f"/repos/{repo_slug(args)}/issues/{number}", {"state": "closed"})
        print(f"Closed issue #{number}: {issue_url(args, int(number))}")
    else:
        print(f"Project item `{item['id']}` has no GitHub issue to close; moving project status only.")

    field_args = argparse.Namespace(**vars(args))
    field_args.status = args.status or "Done"
    apply_and_maybe_verify_project_fields(field_args, client, project, item["id"])
    print(f"Moved `{item['id']}` to {field_args.status}.")


def cmd_rename(args: argparse.Namespace, client: GitHubClient) -> None:
    project = resolve_project(args, client)
    item = resolve_item(list_items(client, project.id), args.item)
    content = item.get("content") or {}
    content_id = content.get("id")
    if not content_id:
        raise GitHubError("Cannot rename an item without content.")

    typename = content.get("__typename")
    body = read_body_arg(args.body, args.body_file)
    if body is None:
        body = content.get("body")
    variables = {"title": args.title, "body": body}
    if typename == "DraftIssue":
        variables["draftIssueId"] = content_id
        client.graphql(UPDATE_DRAFT_MUTATION, variables)
    elif typename == "Issue":
        variables["issueId"] = content_id
        client.graphql(UPDATE_ISSUE_MUTATION, variables)
    else:
        raise GitHubError(f"Rename is only supported for draft issues and issues, not {typename}.")
    print(f"Updated {typename} `{content_id}`.")


def read_body_arg(body: str | None, path: str | None) -> str | None:
    if path:
        if path == "-":
            return sys.stdin.read()
        with open(path, "r", encoding="utf-8") as handle:
            return handle.read()
    if body == "-":
        return sys.stdin.read()
    return body


def add_common_args(parser: argparse.ArgumentParser, suppress_defaults: bool = False) -> None:
    default = argparse.SUPPRESS if suppress_defaults else None
    parser.add_argument(
        "--owner",
        default=default if suppress_defaults else os.environ.get("CODEGYM_GITHUB_OWNER", DEFAULT_OWNER),
    )
    parser.add_argument(
        "--repo-owner",
        default=default if suppress_defaults else os.environ.get("CODEGYM_GITHUB_REPO_OWNER", DEFAULT_OWNER),
    )
    parser.add_argument(
        "--repo",
        default=default if suppress_defaults else os.environ.get("CODEGYM_GITHUB_REPO", DEFAULT_REPO),
    )
    parser.add_argument(
        "--project-number",
        type=int,
        default=default if suppress_defaults else env_int("CODEGYM_GITHUB_PROJECT_NUMBER"),
    )
    parser.add_argument(
        "--project-title",
        default=default if suppress_defaults else os.environ.get("CODEGYM_GITHUB_PROJECT_TITLE", DEFAULT_PROJECT_TITLE),
    )


def env_int(name: str) -> int | None:
    value = os.environ.get(name)
    return int(value) if value else None


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        description="View and manage the Polymarket EV Bot GitHub Projects v2 kanban board.",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog=textwrap.dedent(
            """\
            Examples:
              python .agents/skills/repo-knowledge/scripts/github_project_board.py projects
              python .agents/skills/repo-knowledge/scripts/github_project_board.py columns --project-number 4
              python .agents/skills/repo-knowledge/scripts/github_project_board.py fields --project-number 4
              python .agents/skills/repo-knowledge/scripts/github_project_board.py list --project-number 4
              python .agents/skills/repo-knowledge/scripts/github_project_board.py add-issue 5 --project-number 4 --status Todo --category Performance --priority P1 --size M
              python .agents/skills/repo-knowledge/scripts/github_project_board.py add-draft "Write CI workflow" --project-number 4 --status Todo --category "Project / Process" --priority P2 --size S --body "..."
              python .agents/skills/repo-knowledge/scripts/github_project_board.py create-issue --project-number 4 --title "Write CI workflow" --body-file /tmp/body.md --label area:ci,type:task --status Todo --category "Project / Process" --priority P1 --size M --source "planning import"
              python .agents/skills/repo-knowledge/scripts/github_project_board.py upsert-issue --project-number 4 --title "Write CI workflow" --body-file /tmp/body.md --status Todo --category "Project / Process" --priority P1 --size M --source "planning import" --verify
              python .agents/skills/repo-knowledge/scripts/github_project_board.py import-issues --project-number 4 --file /tmp/issues.json --source "conversation import"
              python .agents/skills/repo-knowledge/scripts/github_project_board.py complete 5 --project-number 4 --source "validated in PR 12"
              python .agents/skills/repo-knowledge/scripts/github_project_board.py edit-issue 5 --project-number 4 --body-file /tmp/body.md --add-label area:backend --status "In Progress" --category "Provider API Integration"
              python .agents/skills/repo-knowledge/scripts/github_project_board.py move 5 "In Progress" --project-number 4
              python .agents/skills/repo-knowledge/scripts/github_project_board.py set-field 5 Priority P1 --project-number 4
              python .agents/skills/repo-knowledge/scripts/github_project_board.py set-fields 5 --status Done --priority P1 --size M --source "validated" --project-number 4
              python .agents/skills/repo-knowledge/scripts/github_project_board.py rename 5 "M1: CI workflow" --project-number 4
            """
        ),
    )
    add_common_args(parser)

    sub = parser.add_subparsers(dest="command", required=True)

    projects = sub.add_parser("projects", help="List owner projects.")
    add_common_args(projects, suppress_defaults=True)
    projects.set_defaults(func=cmd_projects)

    columns = sub.add_parser("columns", help="List project Status column options.")
    add_common_args(columns, suppress_defaults=True)
    columns.set_defaults(func=cmd_columns)

    fields = sub.add_parser("fields", help="List project fields and single-select options.")
    add_common_args(fields, suppress_defaults=True)
    fields.set_defaults(func=cmd_fields)

    list_parser = sub.add_parser("list", help="Render project items as Markdown.")
    add_common_args(list_parser, suppress_defaults=True)
    list_parser.add_argument("--status", help="Only show items in this Status column.")
    list_parser.add_argument("--flat", action="store_true", help="Do not group by Status.")
    list_parser.set_defaults(func=cmd_list)

    show = sub.add_parser("show", help="Expand one issue, draft, or PR as Markdown.")
    add_common_args(show, suppress_defaults=True)
    show.add_argument("item", help="Project item ID, issue number, URL, or exact title.")
    show.set_defaults(func=cmd_show)

    add_issue = sub.add_parser("add-issue", help="Add a repository issue to the project.")
    add_common_args(add_issue, suppress_defaults=True)
    add_issue.add_argument("number", type=int, help="Issue number in --repo.")
    add_issue.add_argument("--status", help="Optional Status column to move into after adding.")
    add_issue.add_argument("--category", help="Optional Category field value.")
    add_issue.add_argument("--priority", help="Optional Priority field value.")
    add_issue.add_argument("--size", help="Optional Size field value.")
    add_issue.add_argument("--source", help="Optional Source field value.")
    add_issue.add_argument("--verify", action="store_true", help="Read back project fields after updating them.")
    add_issue.set_defaults(func=cmd_add_issue)

    add_draft = sub.add_parser("add-draft", help="Add a draft issue to the project.")
    add_common_args(add_draft, suppress_defaults=True)
    add_draft.add_argument("title")
    add_draft.add_argument("--body")
    add_draft.add_argument("--body-file")
    add_draft.add_argument("--status", help="Optional Status column to move into after adding.")
    add_draft.add_argument("--category", help="Optional Category field value.")
    add_draft.add_argument("--priority", help="Optional Priority field value.")
    add_draft.add_argument("--size", help="Optional Size field value.")
    add_draft.add_argument("--source", help="Optional Source field value.")
    add_draft.add_argument("--verify", action="store_true", help="Read back project fields after updating them.")
    add_draft.set_defaults(func=cmd_add_draft)

    create_issue = sub.add_parser("create-issue", help="Create a GitHub issue and optionally add it to the project.")
    add_common_args(create_issue, suppress_defaults=True)
    create_issue.add_argument("--title", required=True)
    create_issue.add_argument("--body", default="")
    create_issue.add_argument("--body-file")
    create_issue.add_argument("--label", action="append", help="Comma-separated label list. May be repeated.")
    create_issue.add_argument("--status", help="Optional Status field value.")
    create_issue.add_argument("--category", help="Optional Category field value.")
    create_issue.add_argument("--priority", help="Optional Priority field value.")
    create_issue.add_argument("--size", help="Optional Size field value.")
    create_issue.add_argument("--source", help="Optional Source field value.")
    create_issue.add_argument("--verify", action="store_true", help="Read back project fields after updating them.")
    create_issue.add_argument("--no-project", action="store_true", help="Create the issue without adding it to the project.")
    create_issue.set_defaults(func=cmd_create_issue)

    upsert_issue = sub.add_parser("upsert-issue", help="Create or update a GitHub issue by exact project title.")
    add_common_args(upsert_issue, suppress_defaults=True)
    upsert_issue.add_argument("--title", required=True)
    upsert_issue.add_argument("--body")
    upsert_issue.add_argument("--body-file")
    upsert_issue.add_argument("--label", action="append", help="Comma-separated label list. May be repeated.")
    upsert_issue.add_argument("--state", choices=["open", "closed"])
    upsert_issue.add_argument("--close", action="store_true", help="Close the issue and default project Status to Done.")
    upsert_issue.add_argument("--status", help="Optional Status field value.")
    upsert_issue.add_argument("--category", help="Optional Category field value.")
    upsert_issue.add_argument("--priority", help="Optional Priority field value.")
    upsert_issue.add_argument("--size", help="Optional Size field value.")
    upsert_issue.add_argument("--source", help="Optional Source field value.")
    upsert_issue.add_argument("--verify", action="store_true", help="Read back project fields after updating them.")
    upsert_issue.add_argument("--dry-run", action="store_true", help="Print intended creates/updates without writing.")
    upsert_issue.set_defaults(func=cmd_upsert_issue)

    import_issues = sub.add_parser(
        "import-issues",
        help="Upsert many GitHub issues from inline JSON, stdin, or a JSON file.",
    )
    add_common_args(import_issues, suppress_defaults=True)
    import_issues.add_argument(
        "--file",
        help="JSON list or object with an `issues` list. Use '-' to read stdin. Omit to read stdin.",
    )
    import_issues.add_argument(
        "--json",
        help="Inline JSON string (list or object with an `issues` list). Avoids creating a temp file.",
    )
    import_issues.add_argument(
        "--stdin",
        action="store_true",
        help="Read JSON from stdin (e.g. via a heredoc).",
    )
    import_issues.add_argument("--label", action="append", help="Default comma-separated labels. Entry `labels` override this.")
    import_issues.add_argument("--status", help="Default Status field value.")
    import_issues.add_argument("--category", help="Default Category field value.")
    import_issues.add_argument("--priority", help="Default Priority field value.")
    import_issues.add_argument("--size", help="Default Size field value.")
    import_issues.add_argument("--source", help="Default Source field value.")
    import_issues.add_argument("--verify", action="store_true", help="Read back project fields after updating them.")
    import_issues.add_argument("--dry-run", action="store_true", help="Print intended creates/updates without writing.")
    import_issues.set_defaults(func=cmd_import_issues)

    edit_issue = sub.add_parser("edit-issue", help="Edit a GitHub issue and optionally update project fields.")
    add_common_args(edit_issue, suppress_defaults=True)
    edit_issue.add_argument("number", type=int)
    edit_issue.add_argument("--title")
    edit_issue.add_argument("--body")
    edit_issue.add_argument("--body-file")
    edit_issue.add_argument("--add-label", action="append", help="Comma-separated label list. May be repeated.")
    edit_issue.add_argument("--state", choices=["open", "closed"])
    edit_issue.add_argument("--status", help="Optional Status field value.")
    edit_issue.add_argument("--category", help="Optional Category field value.")
    edit_issue.add_argument("--priority", help="Optional Priority field value.")
    edit_issue.add_argument("--size", help="Optional Size field value.")
    edit_issue.add_argument("--source", help="Optional Source field value.")
    edit_issue.add_argument("--verify", action="store_true", help="Read back project fields after updating them.")
    edit_issue.set_defaults(func=cmd_edit_issue)

    complete = sub.add_parser("complete", help="Close a GitHub issue and move its project item to Done.")
    add_common_args(complete, suppress_defaults=True)
    complete.add_argument("item", help="Project item ID, issue number, URL, or exact title.")
    complete.add_argument("--status", default="Done", help="Status value to set; defaults to Done.")
    complete.add_argument("--category", help="Optional Category field value.")
    complete.add_argument("--priority", help="Optional Priority field value.")
    complete.add_argument("--size", help="Optional Size field value.")
    complete.add_argument("--source", help="Optional Source field value.")
    complete.add_argument("--verify", action="store_true", help="Read back project fields after updating them.")
    complete.set_defaults(func=cmd_complete)

    delete = sub.add_parser("delete", help="Delete an item from the project board.")
    add_common_args(delete, suppress_defaults=True)
    delete.add_argument("item", help="Project item ID, issue number, URL, or exact title.")
    delete.set_defaults(func=cmd_delete)

    move = sub.add_parser("move", help="Move an item to a Status column.")
    add_common_args(move, suppress_defaults=True)
    move.add_argument("item", help="Project item ID, issue number, URL, or exact title.")
    move.add_argument("status", help="Status column name, e.g. Ready.")
    move.set_defaults(func=cmd_move)

    set_field = sub.add_parser("set-field", help="Set a project field value on an item.")
    add_common_args(set_field, suppress_defaults=True)
    set_field.add_argument("item", help="Project item ID, issue number, URL, or exact title.")
    set_field.add_argument("field", help="Project field name.")
    set_field.add_argument("value", help="Field value. Single-select fields accept option names.")
    set_field.set_defaults(func=cmd_set_field)

    set_fields = sub.add_parser("set-fields", help="Set common project fields on an item.")
    add_common_args(set_fields, suppress_defaults=True)
    set_fields.add_argument("item", help="Project item ID, issue number, URL, or exact title.")
    set_fields.add_argument("--status", help="Optional Status field value.")
    set_fields.add_argument("--category", help="Optional Category field value.")
    set_fields.add_argument("--priority", help="Optional Priority field value.")
    set_fields.add_argument("--size", help="Optional Size field value.")
    set_fields.add_argument("--source", help="Optional Source field value.")
    set_fields.add_argument("--verify", action="store_true", help="Read back project fields after updating them.")
    set_fields.set_defaults(func=cmd_set_fields)

    rename = sub.add_parser("rename", help="Rename an issue or draft issue, optionally replacing body.")
    add_common_args(rename, suppress_defaults=True)
    rename.add_argument("item", help="Project item ID, issue number, URL, or exact title.")
    rename.add_argument("title")
    rename.add_argument("--body")
    rename.add_argument("--body-file")
    rename.set_defaults(func=cmd_rename)

    return parser


def main() -> int:
    parser = build_parser()
    args = parser.parse_args()
    try:
        client = GitHubClient(token_from_env())
        args.func(args, client)
        return 0
    except GitHubError as err:
        print(f"error: {err}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
