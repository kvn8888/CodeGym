import { useEffect, useState, type FormEvent } from 'react';
import { Save } from 'lucide-react';

import { Button } from '@/components/ui/button';
import { Card } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Badge } from '@/components/ui/badge';
import { api } from '../../shared/api/client';
import type { GenAICostAggregate, UpdateUserProfileInput, UserProfile } from '../../shared/api/types';
import { GridSpinner } from '../../shared/components/GridSpinner';

const sourceLabel: Record<UserProfile['display_name_source'], string> = {
  oauth: 'Google',
  user: 'Custom',
  fallback: 'Fallback',
};

function formatUsd(value: number) {
  if (!Number.isFinite(value) || value <= 0) return '$0.00';
  if (value < 0.01) return `$${value.toFixed(4)}`;
  return `$${value.toFixed(2)}`;
}

function formatTokens(value: number) {
  return new Intl.NumberFormat('en', { maximumFractionDigits: 0 }).format(value || 0);
}

export function SettingsPage() {
  const [profile, setProfile] = useState<UserProfile | null>(null);
  const [displayName, setDisplayName] = useState('');
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [saved, setSaved] = useState(false);
  const [cost, setCost] = useState<GenAICostAggregate | null>(null);
  const [costError, setCostError] = useState<string | null>(null);

  useEffect(() => {
    api
      .get<UserProfile>('/me')
      .then((data) => {
        setProfile(data);
        setDisplayName(data.display_name);
        setError(null);
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : 'Could not load settings.');
      })
      .finally(() => setLoading(false));

    // Best-effort: usage is secondary to profile settings.
    api
      .get<GenAICostAggregate>('/cost')
      .then((data) => {
        setCost(data);
        setCostError(null);
      })
      .catch((err: unknown) => {
        setCost(null);
        setCostError(err instanceof Error ? err.message : 'Could not load usage.');
      });
  }, []);

  const handleSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();

    const nextDisplayName = displayName.trim();
    if (!nextDisplayName) {
      setError('Display name is required.');
      return;
    }

    setSaving(true);
    setSaved(false);
    setError(null);

    try {
      const input: UpdateUserProfileInput = { display_name: nextDisplayName };
      const updated = await api.patch<UserProfile>('/me', input);
      setProfile(updated);
      setDisplayName(updated.display_name);
      setSaved(true);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Could not save settings.');
    } finally {
      setSaving(false);
    }
  };

  if (loading) {
    return (
      <div className="flex justify-center py-16">
        <GridSpinner size="md" />
      </div>
    );
  }

  return (
    <div className="mx-auto max-w-3xl px-4 py-6 sm:px-6 lg:py-8">
      <div className="mb-6 border-b pb-5">
        <h1 className="text-2xl leading-8 font-semibold">Settings</h1>
      </div>

      <Card className="gap-0 px-5 py-5">
        <form onSubmit={handleSubmit} className="flex flex-col gap-6">
          <div className="flex flex-col gap-2">
            <Label htmlFor="display-name">Display name</Label>
            <Input
              id="display-name"
              value={displayName}
              maxLength={80}
              onChange={(event) => {
                setDisplayName(event.target.value);
                setSaved(false);
              }}
              autoComplete="name"
            />
          </div>

          <div className="grid gap-4 rounded-xl border border-border bg-muted/30 p-4 text-sm md:grid-cols-2">
            <div>
              <div className="text-muted-foreground text-xs">Email</div>
              <div className="mt-1 truncate font-medium">{profile?.email || 'Not provided'}</div>
            </div>
            <div>
              <div className="text-muted-foreground text-xs">Name source</div>
              <div className="mt-1">
                <Badge variant="secondary">
                  {profile ? sourceLabel[profile.display_name_source] : 'Unknown'}
                </Badge>
              </div>
            </div>
            <div className="md:col-span-2">
              <div className="text-muted-foreground text-xs">Workspace</div>
              <div className="mt-1 truncate font-mono text-xs">{profile?.default_workspace_id || 'Not available'}</div>
            </div>
          </div>

          {error && (
            <div className="border-destructive/40 bg-destructive/10 text-destructive rounded-xl border px-4 py-3 text-sm">
              {error}
            </div>
          )}

          <div className="flex items-center justify-between gap-4">
            <div className="text-muted-foreground text-sm">{saved ? 'Saved.' : ''}</div>
            <Button type="submit" disabled={saving || displayName.trim() === ''}>
              <Save size={16} strokeWidth={1.8} />
              {saving ? 'Saving' : 'Save'}
            </Button>
          </div>
        </form>
      </Card>

      {/* Low-emphasis GenAI usage; failures stay quiet so profile remains primary. */}
      <div className="text-muted-foreground mt-8 space-y-2 border-t border-border/60 pt-6 text-xs">
        <div className="font-medium tracking-wide text-muted-foreground/90 uppercase">
          AI usage
        </div>
        {costError && !cost && (
          <p className="text-muted-foreground/80">Usage unavailable right now.</p>
        )}
        {cost && cost.call_count === 0 && (
          <p>No generation calls recorded in this workspace yet.</p>
        )}
        {cost && cost.call_count > 0 && (
          <>
            <p>
              {formatTokens(cost.total_tokens_in)} in · {formatTokens(cost.total_tokens_out)} out
              · ~{formatUsd(cost.total_cost_usd)} est. · {cost.call_count} call
              {cost.call_count === 1 ? '' : 's'}
            </p>
            {cost.by_provider.length > 0 && (
              <p className="text-muted-foreground/80">
                {cost.by_provider
                  .map(
                    (slice) =>
                      `${slice.provider} ~${formatUsd(slice.cost_usd)} (${slice.call_count})`,
                  )
                  .join(' · ')}
              </p>
            )}
            <p className="text-muted-foreground/70">
              Estimates only (rate card {cost.pricing_as_of}); not a bill.
            </p>
          </>
        )}
      </div>
    </div>
  );
}
