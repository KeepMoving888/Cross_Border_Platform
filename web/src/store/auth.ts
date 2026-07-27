/**
 * 鉴权状态:token、user 信息、登录登出、角色校验
 */
import { create } from 'zustand';
import { persist } from 'zustand/middleware';
import { login as apiLogin, logout as apiLogout } from '@/api/auth';
import { TOKEN_STORAGE_KEY, USER_STORAGE_KEY } from '@/utils/constants';
import type { LoginRequest, UserInfo } from '@/types/api';

interface AuthState {
  token: string | null;
  user: UserInfo | null;
  loading: boolean;
  login: (payload: LoginRequest) => Promise<void>;
  logout: () => Promise<void>;
  isAuthenticated: () => boolean;
  hasRole: (...roles: string[]) => boolean;
  roles: () => string[];
}

function normalizeUser(raw: Record<string, unknown> | UserInfo | null | undefined): UserInfo | null {
  if (!raw) return null;
  const anyRaw = raw as Record<string, unknown>;
  const role = (anyRaw.role as string) || '';
  const rolesFromArr = Array.isArray(anyRaw.roles)
    ? (anyRaw.roles as string[]).filter(Boolean)
    : [];
  const roles = rolesFromArr.length > 0 ? rolesFromArr : role ? [role] : [];
  return {
    id: Number(anyRaw.id || 0),
    username: String(anyRaw.username || ''),
    nickname: String(anyRaw.nickname || anyRaw.real_name || anyRaw.username || ''),
    avatar: anyRaw.avatar as string | undefined,
    email: anyRaw.email as string | undefined,
    role: role || roles[0] || '',
    roles,
    department: anyRaw.department as string | undefined,
  };
}

export const useAuthStore = create<AuthState>()(
  persist(
    (set, get) => ({
      token: localStorage.getItem(TOKEN_STORAGE_KEY),
      user: (() => {
        try {
          const raw = localStorage.getItem(USER_STORAGE_KEY);
          return raw ? normalizeUser(JSON.parse(raw)) : null;
        } catch {
          return null;
        }
      })(),
      loading: false,
      login: async (payload) => {
        set({ loading: true });
        try {
          const res = await apiLogin(payload);
          const anyRes = res as unknown as Record<string, unknown>;
          const user = normalizeUser(
            (anyRes.user as UserInfo) ||
              (anyRes.user_info as UserInfo) ||
              (res as { user?: UserInfo }).user,
          );
          localStorage.setItem(TOKEN_STORAGE_KEY, res.token);
          if (user) {
            localStorage.setItem(USER_STORAGE_KEY, JSON.stringify(user));
          }
          set({ token: res.token, user });
        } finally {
          set({ loading: false });
        }
      },
      logout: async () => {
        await apiLogout();
        localStorage.removeItem(TOKEN_STORAGE_KEY);
        localStorage.removeItem(USER_STORAGE_KEY);
        set({ token: null, user: null });
      },
      isAuthenticated: () => !!get().token,
      roles: () => {
        const user = get().user;
        if (!user) return [];
        if (user.roles?.length) return user.roles;
        return user.role ? [user.role] : [];
      },
      hasRole: (...roles) => {
        const mine = get().roles();
        if (!roles.length) return true;
        return roles.some((r) => mine.includes(r));
      },
    }),
    {
      name: 'cbp-auth-store',
      partialize: (state) => ({ token: state.token, user: state.user }),
    },
  ),
);
