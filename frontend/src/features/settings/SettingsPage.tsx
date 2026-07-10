import { useEffect, useState, type FormEvent } from 'react';
import { Save } from 'lucide-react';

import { Button } from '@/components/ui/button';
import { Card } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Badge } from '@/components/ui/badge';
import { api } from '../../shared/api/client';
import type { UpdateUserProfileInput, UserProfile } from '../../shared/api/types';
import { GridSpinner } from '../../shared/components/GridSpinner';

const sourceLabel: Record<UserProfile['display_name_source'], string> = {
  oauth: 'Google',
  user: 'Custom',
  fallback: 'Fallback',
};

export function SettingsPage() {
  const [profile, setProfile] = useState<UserProfile | null>(null);
  const [displayName, setDisplayName] = useState('');
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [saved, setSaved] = useState(false);

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
    <div className="mx-auto max-w-3xl px-6 py-12">
      <h1 className="mb-8 text-[40px] font-semibold leading-[48px] tracking-[-2.4px]">Settings</h1>

      <Card className="gap-0 px-6 py-6">
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
              <div className="mt-1 truncate font-mono text-xs">{profile?.default_tenant_id || 'Not available'}</div>
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
    </div>
  );
}
