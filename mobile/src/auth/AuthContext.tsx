import * as AuthSession from 'expo-auth-session';
import * as SecureStore from 'expo-secure-store';
import * as WebBrowser from 'expo-web-browser';
import React, { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react';
import { Platform } from 'react-native';

import { authConfig } from './config';

// Required once per app for expo-auth-session's browser-based flow to
// correctly close and return control to the app after Hosted UI redirects
// back — confirmed against the current SDK 57 docs (AGENTS.md's own
// instruction to check them before writing this file). No-op on web/native
// paths that don't use it, harmless to call unconditionally.
WebBrowser.maybeCompleteAuthSession();

const discovery: AuthSession.DiscoveryDocument = {
  authorizationEndpoint: `https://${authConfig.hostedUiDomain}/oauth2/authorize`,
  tokenEndpoint: `https://${authConfig.hostedUiDomain}/oauth2/token`,
  revocationEndpoint: `https://${authConfig.hostedUiDomain}/oauth2/revoke`,
};

// expo-auth-session's makeRedirectUri() only appends a `path` option on
// web/Expo Go, never on native — so native gets the bare "jobsyllabus://"
// scheme (matching infra/terraform/envs/dev-compute/main.tf's Cognito
// callback_urls exactly), while web needs an explicitly hard-coded exact
// origin rather than an auto-detected one, per the SDK's own
// recommendation for production web redirects (see config.ts's comment).
const redirectUri =
  Platform.OS === 'web' ? authConfig.webRedirectUri : AuthSession.makeRedirectUri({ scheme: 'jobsyllabus' });

// SecureStore, never AsyncStorage — docs/design.md §8 is explicit about
// this for token storage. Web has no SecureStore backing (no Keychain/
// Keystore to wrap), so it falls back to localStorage there; tokens
// living in a web page's storage is an accepted, ordinary web-app
// tradeoff, not a regression — there's no more-private per-origin storage
// a browser offers.
const secureStoreAvailable = Platform.OS !== 'web';

async function storeItem(key: string, value: string): Promise<void> {
  if (secureStoreAvailable) {
    await SecureStore.setItemAsync(key, value);
  } else {
    localStorage.setItem(key, value);
  }
}

async function readItem(key: string): Promise<string | null> {
  if (secureStoreAvailable) {
    return SecureStore.getItemAsync(key);
  }
  return localStorage.getItem(key);
}

async function deleteItem(key: string): Promise<void> {
  if (secureStoreAvailable) {
    await SecureStore.deleteItemAsync(key);
  } else {
    localStorage.removeItem(key);
  }
}

interface StoredTokens {
  accessToken: string;
  refreshToken?: string;
  expiresAt: number; // epoch ms
}

const TOKENS_KEY = 'job-syllabus-tokens';
const PKCE_VERIFIER_KEY = 'job-syllabus-pkce-verifier';

interface AuthContextValue {
  isSignedIn: boolean;
  isLoading: boolean;
  accessToken: string | null;
  signIn: () => Promise<void>;
  signOut: () => Promise<void>;
}

const AuthContext = createContext<AuthContextValue | undefined>(undefined);

async function tokensFromCodeExchange(code: string, codeVerifier: string): Promise<StoredTokens> {
  const result = await AuthSession.exchangeCodeAsync(
    {
      clientId: authConfig.clientId,
      code,
      redirectUri,
      extraParams: { code_verifier: codeVerifier },
    },
    discovery,
  );
  return {
    accessToken: result.accessToken,
    refreshToken: result.refreshToken,
    expiresAt: Date.now() + (result.expiresIn ?? 3600) * 1000,
  };
}

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [tokens, setTokens] = useState<StoredTokens | null>(null);
  const [isLoading, setIsLoading] = useState(true);

  // Native only: expo-auth-session's own popup/in-app-browser flow. Web
  // deliberately does NOT use promptAsync — see signIn() below for why.
  const [request, response, promptAsync] = AuthSession.useAuthRequest(
    {
      clientId: authConfig.clientId,
      redirectUri,
      scopes: authConfig.scopes,
      usePKCE: true,
      responseType: AuthSession.ResponseType.Code,
    },
    discovery,
  );

  // Restore a persisted session on launch — or, on web, first check
  // whether this load IS the redirect back from Hosted UI (a `code` query
  // param present in the URL) before falling back to a stored session.
  useEffect(() => {
    (async () => {
      if (Platform.OS === 'web') {
        const params = new URLSearchParams(window.location.search);
        const code = params.get('code');
        if (code) {
          const codeVerifier = await readItem(PKCE_VERIFIER_KEY);
          // Always scrub the code out of the URL, whether or not the
          // exchange below succeeds — leaving it there lets a page
          // refresh replay (and fail) the same one-time-use code.
          window.history.replaceState({}, '', window.location.pathname);
          if (codeVerifier) {
            try {
              const next = await tokensFromCodeExchange(code, codeVerifier);
              await storeItem(TOKENS_KEY, JSON.stringify(next));
              await deleteItem(PKCE_VERIFIER_KEY);
              setTokens(next);
              setIsLoading(false);
              return;
            } catch {
              await deleteItem(PKCE_VERIFIER_KEY);
              // Fall through to the stored-session check below — a failed
              // exchange shouldn't be worse than "not signed in yet."
            }
          }
        }
      }

      const raw = await readItem(TOKENS_KEY);
      if (raw) {
        const parsed: StoredTokens = JSON.parse(raw);
        if (parsed.expiresAt > Date.now()) {
          setTokens(parsed);
        } else if (parsed.refreshToken) {
          try {
            const refreshed = await AuthSession.refreshAsync(
              { clientId: authConfig.clientId, refreshToken: parsed.refreshToken },
              discovery,
            );
            const next: StoredTokens = {
              accessToken: refreshed.accessToken,
              refreshToken: refreshed.refreshToken ?? parsed.refreshToken,
              expiresAt: Date.now() + (refreshed.expiresIn ?? 3600) * 1000,
            };
            await storeItem(TOKENS_KEY, JSON.stringify(next));
            setTokens(next);
          } catch {
            await deleteItem(TOKENS_KEY);
          }
        } else {
          await deleteItem(TOKENS_KEY);
        }
      }
      setIsLoading(false);
    })();
  }, []);

  // Native only: exchange the authorization code once Hosted UI's in-app
  // browser redirects back with a `response`. Web's equivalent is the
  // `code` query-param branch in the effect above, not this one.
  useEffect(() => {
    if (Platform.OS === 'web' || response?.type !== 'success' || !request?.codeVerifier) {
      return;
    }
    (async () => {
      const next = await tokensFromCodeExchange(response.params.code, request.codeVerifier!);
      await storeItem(TOKENS_KEY, JSON.stringify(next));
      setTokens(next);
    })();
  }, [response, request]);

  const signIn = useCallback(async () => {
    if (Platform.OS === 'web') {
      // Full-page redirect, not promptAsync()'s popup — expo-auth-session's
      // web popup flow reproducibly triggers the browser's own popup
      // blocker ("Popup window was blocked... invoked too long after a
      // user input was fired"), confirmed against a real click in a real
      // browser during Phase 6 development, not just reasoned about. A
      // redirect has no popup to block, and is the more conventional
      // pattern for web OAuth anyway. The PKCE code_verifier has to
      // survive the full navigation, hence storing it (not keeping it in
      // memory the way the native path can via `request`).
      const authRequest = new AuthSession.AuthRequest({
        clientId: authConfig.clientId,
        redirectUri,
        scopes: authConfig.scopes,
        usePKCE: true,
        responseType: AuthSession.ResponseType.Code,
      });
      const authUrl = await authRequest.makeAuthUrlAsync(discovery);
      if (authRequest.codeVerifier) {
        await storeItem(PKCE_VERIFIER_KEY, authRequest.codeVerifier);
      }
      window.location.href = authUrl;
      return;
    }
    await promptAsync();
  }, [promptAsync]);

  const signOut = useCallback(async () => {
    await deleteItem(TOKENS_KEY);
    await deleteItem(PKCE_VERIFIER_KEY);
    setTokens(null);
    // Hosted UI's own logout endpoint clears its session cookie too —
    // without this, a subsequent signIn() would silently re-auth without
    // showing the login form, which is surprising on a shared device.
    const logoutUrl = `https://${authConfig.hostedUiDomain}/logout?client_id=${authConfig.clientId}&logout_uri=${encodeURIComponent(redirectUri)}`;
    if (Platform.OS === 'web') {
      window.location.href = logoutUrl;
    } else {
      await WebBrowser.openAuthSessionAsync(logoutUrl, redirectUri);
    }
  }, []);

  const value = useMemo<AuthContextValue>(
    () => ({
      isSignedIn: tokens !== null,
      isLoading,
      accessToken: tokens?.accessToken ?? null,
      signIn,
      signOut,
    }),
    [tokens, isLoading, signIn, signOut],
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext);
  if (!ctx) {
    throw new Error('useAuth must be used within an AuthProvider');
  }
  return ctx;
}
