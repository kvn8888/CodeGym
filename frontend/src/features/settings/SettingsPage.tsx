import { useEffect, useState, type FormEvent } from 'react';
import { Save } from 'lucide-react';

import { Button } from '@/components/ui/button';
import { Card } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Badge } from '@/components/ui/badge';
import type { UserProfile } from '../../shared/api/types';
import { useAccountProfile } from '../../shared/auth/accountProfileStore';
import { GridSpinner } from '../../shared/components/GridSpinner';

const sourceLabel: Record<UserProfile['display_name_source'], string> = {
  oauth: 'Google',
  user: 'Custom',
  fallback: 'Fallback',
};

export function SettingsPage() {
  const profile = useAccountProfile((state) => state.profile);
  const loading = useAccountProfile((state) => state.loading);
  const profileError = useAccountProfile((state) => state.error);
  const loadProfile = useAccountProfile((state) => state.loadProfile);

  useEffect(() => {
    void loadProfile();
  }, [loadProfile]);

  if (loading || (!profile && !profileError)) {
    return (
      <div className="flex justify-center py-16">
        <GridSpinner size="md" />
      </div>
    );
  }

  if (!profile) {
    return (
      <div className="mx-auto max-w-3xl px-6 py-12">
        <h1 className="mb-8 text-[40px] font-semibold leading-[48px] tracking-[-2.4px]">Settings</h1>
        <Card className="items-start gap-4 px-6 py-6">
          <p className="text-destructive text-sm">{profileError || 'Could not load settings.'}</p>
          <Button type="button" variant="outline" onClick={() => void loadProfile(true)}>
            Try again
          </Button>
        </Card>
      </div>
    );
  }

  return <SettingsForm profile={profile} profileError={profileError} />;
}

function SettingsForm({
  profile,
  profileError,
}: {
  profile: UserProfile;
  profileError: string | null;
}) {
  const updateDisplayName = useAccountProfile((state) => state.updateDisplayName);
  const [displayName, setDisplayName] = useState(profile.display_name);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [saved, setSaved] = useState(false);

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
      const updated = await updateDisplayName(nextDisplayName);
      setDisplayName(updated.display_name);
      setSaved(true);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Could not save settings.');
    } finally {
      setSaving(false);
    }
  };

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
                setError(null);
              }}
              autoComplete="name"
            />
          </div>

          <div className="grid gap-4 rounded-xl border border-border bg-muted/30 p-4 text-sm md:grid-cols-2">
            <div className="min-w-0">
              <div className="text-muted-foreground text-xs">Email</div>
              <div className="mt-1 truncate font-medium">{profile.email || 'Not provided'}</div>
            </div>
            <div className="min-w-0">
              <div className="text-muted-foreground text-xs">Name source</div>
              <div className="mt-1">
                <Badge variant="secondary">
                  {sourceLabel[profile.display_name_source]}
                </Badge>
              </div>
            </div>
            <div className="min-w-0 md:col-span-2">
              <div className="text-muted-foreground text-xs">Workspace</div>
              <div className="mt-1 break-all font-mono text-xs">{profile.default_workspace_id || 'Not available'}</div>
            </div>
          </div>

          {(error || profileError) && (
            <div className="border-destructive/40 bg-destructive/10 text-destructive rounded-xl border px-4 py-3 text-sm">
              {error || profileError}
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
