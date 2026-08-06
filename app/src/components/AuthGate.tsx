import { useEffect, useState, type ReactNode, type FormEvent } from 'react';
import { Sparkles, Loader2, ShieldCheck, UserPlus, LogIn } from 'lucide-react';
import { api, getToken, clearSession } from '@/lib/api';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { toast } from 'sonner';

// AuthGate talks to the Go control plane's /auth/* surface. If the backend
// doesn't implement it (e.g., the Node reference server), authStatus 404s and
// we pass through — so both backends work with the same UI.

type Mode = 'loading' | 'open' | 'setup' | 'login';

export default function AuthGate({ children }: { children: ReactNode }) {
  const [mode, setMode] = useState<Mode>('loading');

  useEffect(() => {
    (async () => {
      try {
        const s = await api.authStatus();
        if (s.setupRequired) {
          setMode('setup');
          return;
        }
        if (s.authRequired) {
          if (getToken()) {
            try {
              await api.authMe();
              setMode('open');
              return;
            } catch {
              clearSession();
            }
          }
          setMode('login');
          return;
        }
        setMode('open');
      } catch {
        // auth not supported by this backend -> proceed unauthenticated
        setMode('open');
      }
    })();
  }, []);

  if (mode === 'loading') return <Splash msg="Connecting to FlowForge…" />;
  if (mode === 'setup') return <AuthForm kind="setup" onDone={() => setMode('open')} />;
  if (mode === 'login') return <AuthForm kind="login" onDone={() => setMode('open')} />;
  return <>{children}</>;
}

function Splash({ msg }: { msg: string }) {
  return (
    <div className="flex min-h-screen items-center justify-center bg-slate-50 text-slate-500">
      <div className="flex items-center gap-2 text-sm"><Loader2 className="h-4 w-4 animate-spin" /> {msg}</div>
    </div>
  );
}

function AuthForm({ kind, onDone }: { kind: 'setup' | 'login'; onDone: () => void }) {
  const [username, setUsername] = useState('admin');
  const [password, setPassword] = useState('');
  const [busy, setBusy] = useState(false);
  const isSetup = kind === 'setup';

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    if (username.trim().length < 3 || password.length < 6) {
      toast.error('Username ≥ 3 chars, password ≥ 6 chars');
      return;
    }
    setBusy(true);
    try {
      await (isSetup ? api.authSetup(username.trim(), password) : api.authLogin(username.trim(), password));
      toast.success(isSetup ? 'Admin created — welcome to FlowForge' : `Signed in as ${username.trim()}`);
      onDone();
    } catch (err) {
      toast.error('Could not continue', { description: String((err as Error).message ?? err) });
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="flex min-h-screen items-center justify-center bg-gradient-to-br from-violet-50 via-slate-50 to-emerald-50 px-4">
      <form onSubmit={submit} className="w-full max-w-sm rounded-2xl border bg-white p-7 shadow-xl">
        <div className="mb-5 flex items-center gap-2.5">
          <span className="inline-flex h-9 w-9 items-center justify-center rounded-xl border bg-violet-100 border-violet-200 text-violet-700">
            {isSetup ? <UserPlus className="h-5 w-5" /> : <LogIn className="h-5 w-5" />}
          </span>
          <div>
            <div className="text-base font-bold tracking-tight">{isSetup ? 'Create an admin account' : 'Sign in to FlowForge'}</div>
            <div className="text-[11px] text-muted-foreground">{isSetup ? 'First-run setup — this becomes your administrator login.' : 'Enter your credentials to continue.'}</div>
          </div>
        </div>

        <label className="text-xs font-semibold text-muted-foreground">Username</label>
        <Input value={username} onChange={(e) => setUsername(e.target.value)} className="mt-1 mb-3 h-10" autoComplete="username" />

        <label className="text-xs font-semibold text-muted-foreground">Password</label>
        <Input type="password" value={password} onChange={(e) => setPassword(e.target.value)} className="mt-1 mb-4 h-10" autoComplete={isSetup ? 'new-password' : 'current-password'} />

        <Button type="submit" disabled={busy} className="w-full gap-2 bg-violet-600 hover:bg-violet-700">
          {busy ? <Loader2 className="h-4 w-4 animate-spin" /> : isSetup ? <UserPlus className="h-4 w-4" /> : <LogIn className="h-4 w-4" />}
          {isSetup ? 'Create account & continue' : 'Sign in'}
        </Button>

        <div className="mt-5 flex items-center gap-2 rounded-lg bg-muted/50 px-3 py-2 text-[11px] text-muted-foreground">
          <ShieldCheck className="h-3.5 w-3.5 shrink-0 text-emerald-600" />
          Credentials are stored locally on this server (bcrypt-hashed). Sessions are signed tokens.
        </div>
        <div className="mt-3 flex items-center justify-center gap-1.5 text-[11px] text-slate-400">
          <Sparkles className="h-3 w-3" /> FlowForge · self-hosted workflow platform
        </div>
      </form>
    </div>
  );
}
