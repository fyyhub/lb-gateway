import {
  Activity,
  Braces,
  CheckCircle2,
  ClipboardCheck,
  Database,
  FileText,
  Gauge,
  GitBranch,
  KeyRound,
  LogOut,
  Map as MapIcon,
  Pencil,
  Play,
  Plus,
  RefreshCw,
  Route as RouteIcon,
  Save,
  Search,
  Server,
  Shield,
  ShieldCheck,
  Trash2,
  Wand2,
  XCircle,
} from "lucide-react";
import { FormEvent, useCallback, useEffect, useMemo, useState } from "react";

type NoticeType = "success" | "error";
type Notice = { type: NoticeType; text: string } | null;
type TabID = "dashboard" | "routes" | "upstreams" | "mapping" | "debug" | "logs" | "audit" | "account";

type AdminUser = {
  id?: string;
  username: string;
  role?: string;
  enabled?: boolean;
};

type LoginResponse = {
  token: string;
  user: AdminUser;
};

type RouteConfig = {
  id?: string;
  name: string;
  enabled: boolean;
  priority: number;
  type: "proxy" | "redirect";
  match: MatchConfig;
  upstreamGroupId?: string;
  upstreamGroup?: UpstreamGroup;
  requestRewrite?: RewriteRule[];
  responseMapping?: MappingRule[];
  redirect?: RedirectConfig;
  maxResponseBytes?: number;
};

type MatchConfig = {
  host?: string;
  path: string;
  methods?: string[];
};

type UpstreamGroup = {
  id?: string;
  name?: string;
  strategy: string;
  targets: TargetConfig[];
};

type TargetConfig = {
  id?: string;
  groupId?: string;
  url: string;
  weight: number;
  enabled: boolean;
  healthStatus?: string;
};

type RedirectConfig = {
  statusCode: number;
  strategy: string;
  targets: TargetConfig[];
};

type RewriteRule = {
  type: string;
  key?: string;
  value?: unknown;
  from?: string;
  to?: string;
};

type MappingRule = {
  from?: string;
  to: string;
  value?: unknown;
};

type DebugResult = {
  statusCode: number;
  durationMs: number;
  headers: Record<string, string[]>;
  body: string;
  truncated: boolean;
};

type RequestLog = {
  id: string;
  requestId: string;
  method: string;
  path: string;
  routeId?: string;
  upstreamUrl?: string;
  statusCode: number;
  durationMs: number;
  clientIp: string;
  error?: string;
  createdAt: string;
};

type AuditLog = {
  id: string;
  adminUserId?: string;
  action: string;
  resourceType: string;
  resourceId?: string;
  detail: Record<string, unknown>;
  createdAt: string;
};

type AuthedRequest = <T>(path: string, init?: RequestInit) => Promise<T>;

const tokenKey = "light-api-gateway-admin-token";
const userKey = "light-api-gateway-admin-user";
const baseURLKey = "light-api-gateway-admin-api-base";
const methods = ["GET", "POST", "PUT", "PATCH", "DELETE"];
const strategies = ["round-robin", "weighted-round-robin", "random"];
const rewriteTypes = ["setHeader", "setQuery", "rewritePath", "setJsonBody"];

const sampleSource = `{
  "result": {
    "username": "Alice",
    "userId": 99
  }
}`;

const sampleHeaders = `{
  "X-Debug-Source": "admin-ui"
}`;

function App() {
  const [apiBase, setApiBase] = useState(() => {
    // Default to same-origin ("") so the all-in-one server is called at
    // /admin/api/* on the current host. VITE_ADMIN_API_URL or the in-app field
    // can still point the UI at a separately deployed admin API.
    return (
      localStorage.getItem(baseURLKey) ||
      import.meta.env.VITE_ADMIN_API_URL ||
      ""
    );
  });
  const [token, setToken] = useState(() => localStorage.getItem(tokenKey) || "");
  const [user, setUser] = useState<AdminUser | null>(() => readStoredUser());
  const [routes, setRoutes] = useState<RouteConfig[]>([]);
  const [groups, setGroups] = useState<UpstreamGroup[]>([]);
  const [activeTab, setActiveTab] = useState<TabID>("dashboard");
  const [notice, setNotice] = useState<Notice>(null);
  const [loading, setLoading] = useState(false);
  const [routeDraft, setRouteDraft] = useState<RouteConfig>(() => newRouteDraft());

  const notify = useCallback((text: string, type: NoticeType = "success") => {
    setNotice({ text, type });
    window.setTimeout(() => setNotice(null), 3600);
  }, []);

  const request = useCallback<AuthedRequest>(
    (path, init) => apiFetch(normalizeBaseURL(apiBase), token, path, init),
    [apiBase, token],
  );

  const loadData = useCallback(async () => {
    if (!token) {
      return;
    }
    setLoading(true);
    try {
      const [routeList, groupList] = await Promise.all([
        request<RouteConfig[]>("/admin/api/routes"),
        request<UpstreamGroup[]>("/admin/api/upstream-groups"),
      ]);
      setRoutes(routeList);
      setGroups(groupList);
      setRouteDraft((current) => {
        if (current.id) {
          return current;
        }
        return { ...current, upstreamGroupId: current.upstreamGroupId || groupList[0]?.id };
      });
    } catch (error) {
      notify(errorMessage(error), "error");
    } finally {
      setLoading(false);
    }
  }, [notify, request, token]);

  useEffect(() => {
    void loadData();
  }, [loadData]);

  const handleLogin = async (username: string, password: string, nextBaseURL: string) => {
    const normalized = normalizeBaseURL(nextBaseURL);
    const response = await apiFetch<LoginResponse>(normalized, "", "/admin/api/auth/login", {
      method: "POST",
      body: JSON.stringify({ username, password }),
    });
    localStorage.setItem(tokenKey, response.token);
    localStorage.setItem(userKey, JSON.stringify(response.user));
    localStorage.setItem(baseURLKey, normalized);
    setApiBase(normalized);
    setToken(response.token);
    setUser(response.user);
    notify("登录成功");
  };

  const logout = () => {
    localStorage.removeItem(tokenKey);
    localStorage.removeItem(userKey);
    setToken("");
    setUser(null);
  };

  const saveAPIBase = () => {
    localStorage.setItem(baseURLKey, normalizeBaseURL(apiBase));
    setApiBase(normalizeBaseURL(apiBase));
    notify("管理 API 地址已保存");
  };

  const saveRoute = async (draft: RouteConfig) => {
    const payload = cleanRoute(draft);
    if (draft.id) {
      await request<RouteConfig>(`/admin/api/routes/${encodeURIComponent(draft.id)}`, {
        method: "PUT",
        body: JSON.stringify(payload),
      });
      notify("路由已更新");
    } else {
      const created = await request<RouteConfig>("/admin/api/routes", {
        method: "POST",
        body: JSON.stringify(payload),
      });
      setRouteDraft(cloneRoute(created));
      notify("路由已创建");
    }
    await loadData();
  };

  const deleteRoute = async (route: RouteConfig) => {
    if (!route.id || !window.confirm(`确定删除路由「${route.name}」吗？`)) {
      return;
    }
    await request<void>(`/admin/api/routes/${encodeURIComponent(route.id)}`, { method: "DELETE" });
    setRouteDraft(newRouteDraft(groups));
    notify("路由已删除");
    await loadData();
  };

  const setRouteEnabled = async (route: RouteConfig, enabled: boolean) => {
    if (!route.id) {
      return;
    }
    await request<RouteConfig>(`/admin/api/routes/${encodeURIComponent(route.id)}/enabled`, {
      method: "PATCH",
      body: JSON.stringify({ enabled }),
    });
    notify(enabled ? "路由已启用" : "路由已停用");
    await loadData();
  };

  const onAccountUpdated = useCallback((updated: AdminUser) => {
    localStorage.setItem(userKey, JSON.stringify(updated));
    setUser(updated);
  }, []);

  if (!token) {
    return <LoginScreen apiBase={apiBase} onLogin={handleLogin} />;
  }

  return (
    <div className="app-shell">
      <aside className="sidebar">
        <div className="brand">
          <div className="brand-mark">
            <GitBranch size={22} aria-hidden="true" />
          </div>
          <div>
            <strong>Light Gateway</strong>
            <span>管理控制台</span>
          </div>
        </div>
        <nav className="nav-list" aria-label="主导航">
          {[
            { id: "dashboard", label: "仪表盘", icon: Activity },
            { id: "routes", label: "路由管理", icon: RouteIcon },
            { id: "upstreams", label: "上游服务", icon: Server },
            { id: "mapping", label: "响应映射", icon: MapIcon },
            { id: "debug", label: "调试控制台", icon: Search },
            { id: "logs", label: "请求日志", icon: FileText },
            { id: "audit", label: "审计日志", icon: ShieldCheck },
            { id: "account", label: "账户", icon: KeyRound },
          ].map((item) => {
            const Icon = item.icon;
            return (
              <button
                className={activeTab === item.id ? "nav-item active" : "nav-item"}
                key={item.id}
                onClick={() => setActiveTab(item.id as TabID)}
                type="button"
              >
                <Icon size={18} aria-hidden="true" />
                <span>{item.label}</span>
              </button>
            );
          })}
        </nav>
      </aside>

      <main className="main-area">
        <header className="topbar">
          <div>
            <p className="eyebrow">运行时配置</p>
            <h1>{tabTitle(activeTab)}</h1>
          </div>
          <div className="topbar-actions">
            <label className="base-url">
              <span>管理 API</span>
              <input
                value={apiBase}
                onChange={(event) => setApiBase(event.target.value)}
                placeholder="同源（留空即可）"
              />
            </label>
            <button className="icon-button" onClick={saveAPIBase} title="保存 API 地址" type="button">
              <ClipboardCheck size={18} aria-hidden="true" />
            </button>
            <button className="icon-button" onClick={() => void loadData()} title="刷新" type="button">
              <RefreshCw size={18} aria-hidden="true" className={loading ? "spin" : ""} />
            </button>
            <div className="user-chip">
              <Shield size={16} aria-hidden="true" />
              <span>{user?.username || "admin"}</span>
            </div>
            <button className="icon-button" onClick={logout} title="退出登录" type="button">
              <LogOut size={18} aria-hidden="true" />
            </button>
          </div>
        </header>

        {notice ? <div className={`notice ${notice.type}`}>{notice.text}</div> : null}

        {activeTab === "dashboard" ? <Dashboard routes={routes} groups={groups} /> : null}
        {activeTab === "routes" ? (
          <RoutesPanel
            groups={groups}
            routeDraft={routeDraft}
            routes={routes}
            onDelete={(route) => void deleteRoute(route)}
            onEdit={(route) => setRouteDraft(cloneRoute(route))}
            onNew={() => setRouteDraft(newRouteDraft(groups))}
            onSave={(draft) => void saveRoute(draft)}
            onToggle={(route, enabled) => void setRouteEnabled(route, enabled)}
            setRouteDraft={setRouteDraft}
          />
        ) : null}
        {activeTab === "upstreams" ? (
          <UpstreamsPanel groups={groups} notify={notify} reload={loadData} request={request} />
        ) : null}
        {activeTab === "mapping" ? <MappingPanel notify={notify} request={request} /> : null}
        {activeTab === "debug" ? <DebugPanel notify={notify} request={request} /> : null}
        {activeTab === "logs" ? <LogsPanel notify={notify} request={request} /> : null}
        {activeTab === "audit" ? <AuditPanel notify={notify} request={request} /> : null}
        {activeTab === "account" ? (
          <AccountPanel notify={notify} request={request} user={user} onUpdated={onAccountUpdated} />
        ) : null}
      </main>
    </div>
  );
}

function LoginScreen({
  apiBase,
  onLogin,
}: {
  apiBase: string;
  onLogin: (username: string, password: string, apiBase: string) => Promise<void>;
}) {
  const [username, setUsername] = useState("admin");
  const [password, setPassword] = useState("");
  const [baseURL, setBaseURL] = useState(apiBase);
  const [error, setError] = useState("");
  const [submitting, setSubmitting] = useState(false);

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    setError("");
    setSubmitting(true);
    try {
      await onLogin(username, password, baseURL);
    } catch (loginError) {
      setError(errorMessage(loginError));
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <main className="login-screen">
      <section className="login-panel" aria-labelledby="login-title">
        <div className="brand login-brand">
          <div className="brand-mark">
            <GitBranch size={22} aria-hidden="true" />
          </div>
          <div>
            <strong>Light Gateway</strong>
            <span>管理控制台</span>
          </div>
        </div>
        <form onSubmit={submit} className="login-form">
          <h1 id="login-title">登录</h1>
          <label>
            <span>管理 API</span>
            <input
              value={baseURL}
              onChange={(event) => setBaseURL(event.target.value)}
              placeholder="同源（留空即可）"
            />
          </label>
          <label>
            <span>用户名</span>
            <input autoComplete="username" value={username} onChange={(event) => setUsername(event.target.value)} />
          </label>
          <label>
            <span>密码</span>
            <input
              autoComplete="current-password"
              type="password"
              value={password}
              onChange={(event) => setPassword(event.target.value)}
            />
          </label>
          {error ? <p className="form-error">{error}</p> : null}
          <button className="primary-button" disabled={submitting} type="submit">
            <Shield size={17} aria-hidden="true" />
            <span>{submitting ? "登录中" : "登录"}</span>
          </button>
        </form>
      </section>
    </main>
  );
}

function Dashboard({ routes, groups }: { routes: RouteConfig[]; groups: UpstreamGroup[] }) {
  const targets = groups.flatMap((group) => group.targets);
  const enabledRoutes = routes.filter((route) => route.enabled).length;
  const proxyRoutes = routes.filter((route) => route.type === "proxy").length;
  const mappingRules = routes.reduce((total, route) => total + (route.responseMapping?.length || 0), 0);
  const enabledTargets = targets.filter((target) => target.enabled).length;

  return (
    <div className="dashboard-grid">
      <Metric label="路由总数" value={routes.length} detail={`${enabledRoutes} 条已启用`} icon={RouteIcon} />
      <Metric label="上游分组" value={groups.length} detail={`${enabledTargets} 个启用目标`} icon={Server} />
      <Metric label="代理路由" value={proxyRoutes} detail={`${routes.length - proxyRoutes} 条跳转路由`} icon={Gauge} />
      <Metric label="映射规则" value={mappingRules} detail="响应结构转换" icon={Wand2} />

      <section className="surface wide">
        <div className="section-title">
          <h2>路由优先级</h2>
          <span>{enabledRoutes}/{routes.length} 已启用</span>
        </div>
        <div className="compact-list">
          {routes.map((route) => (
            <div className="compact-row" key={route.id || route.name}>
              <div>
                <strong>{route.name}</strong>
                <span>{route.match.path}</span>
              </div>
              <div className="row-meta">
                <span className={`pill ${route.enabled ? "ok" : "muted"}`}>{formatEnabled(route.enabled)}</span>
                <span className="pill">{formatRouteType(route.type)}</span>
                <span className="pill">P{route.priority}</span>
              </div>
            </div>
          ))}
          {routes.length === 0 ? <EmptyLine text="暂无路由" /> : null}
        </div>
      </section>

      <section className="surface">
        <div className="section-title">
          <h2>上游目标</h2>
          <span>共 {targets.length} 个</span>
        </div>
        <div className="compact-list">
          {groups.map((group) => (
            <div className="compact-row" key={group.id || group.name}>
              <div>
                <strong>{group.name || group.id}</strong>
                <span>{formatStrategy(group.strategy)}</span>
              </div>
              <div className="row-meta">
                <span className="pill">{group.targets.length} 个目标</span>
              </div>
            </div>
          ))}
          {groups.length === 0 ? <EmptyLine text="暂无上游分组" /> : null}
        </div>
      </section>
    </div>
  );
}

function Metric({
  label,
  value,
  detail,
  icon: Icon,
}: {
  label: string;
  value: number;
  detail: string;
  icon: typeof Activity;
}) {
  return (
    <section className="metric">
      <Icon size={21} aria-hidden="true" />
      <div>
        <span>{label}</span>
        <strong>{value}</strong>
        <small>{detail}</small>
      </div>
    </section>
  );
}

function RoutesPanel({
  groups,
  routes,
  routeDraft,
  setRouteDraft,
  onNew,
  onEdit,
  onSave,
  onDelete,
  onToggle,
}: {
  groups: UpstreamGroup[];
  routes: RouteConfig[];
  routeDraft: RouteConfig;
  setRouteDraft: (route: RouteConfig | ((current: RouteConfig) => RouteConfig)) => void;
  onNew: () => void;
  onEdit: (route: RouteConfig) => void;
  onSave: (route: RouteConfig) => void;
  onDelete: (route: RouteConfig) => void;
  onToggle: (route: RouteConfig, enabled: boolean) => void;
}) {
  const updateDraft = (patch: Partial<RouteConfig>) => setRouteDraft((current) => ({ ...current, ...patch }));
  const updateMatch = (patch: Partial<MatchConfig>) =>
    setRouteDraft((current) => ({ ...current, match: { ...current.match, ...patch } }));

  const toggleMethod = (method: string) => {
    setRouteDraft((current) => {
      const currentMethods = current.match.methods || [];
      const nextMethods = currentMethods.includes(method)
        ? currentMethods.filter((item) => item !== method)
        : [...currentMethods, method];
      return { ...current, match: { ...current.match, methods: nextMethods } };
    });
  };

  return (
    <div className="split-layout">
      <section className="surface">
        <div className="section-title">
          <h2>已配置路由</h2>
          <button className="primary-button compact" onClick={onNew} type="button">
            <Plus size={16} aria-hidden="true" />
            <span>路由</span>
          </button>
        </div>
        <div className="table-wrap">
          <table>
            <thead>
              <tr>
                <th>名称</th>
                <th>匹配条件</th>
                <th>目标</th>
                <th>优先级</th>
                <th>状态</th>
                <th aria-label="操作" />
              </tr>
            </thead>
            <tbody>
              {routes.map((route) => (
                <tr key={route.id || route.name}>
                  <td>
                    <strong>{route.name}</strong>
                    <span className="subtext">{formatRouteType(route.type)}</span>
                  </td>
                  <td>
                    <code>{route.match.path}</code>
                    <span className="subtext">{(route.match.methods || []).join(", ") || "全部方法"}</span>
                  </td>
                  <td>{route.type === "proxy" ? route.upstreamGroupId || "内嵌分组" : formatStrategy(route.redirect?.strategy || "")}</td>
                  <td>{route.priority}</td>
                  <td>
                    <button
                      className={`status-toggle ${route.enabled ? "enabled" : ""}`}
                      onClick={() => onToggle(route, !route.enabled)}
                      type="button"
                    >
                      {route.enabled ? <CheckCircle2 size={15} /> : <XCircle size={15} />}
                      <span>{formatEnabled(route.enabled)}</span>
                    </button>
                  </td>
                  <td>
                    <div className="icon-row">
                      <button className="icon-button" onClick={() => onEdit(route)} title="编辑路由" type="button">
                        <Pencil size={16} aria-hidden="true" />
                      </button>
                      <button className="icon-button danger" onClick={() => onDelete(route)} title="删除路由" type="button">
                        <Trash2 size={16} aria-hidden="true" />
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
              {routes.length === 0 ? (
                <tr>
                  <td colSpan={6}>
                    <EmptyLine text="暂无路由配置" />
                  </td>
                </tr>
              ) : null}
            </tbody>
          </table>
        </div>
      </section>

      <section className="surface editor">
        <div className="section-title">
          <h2>{routeDraft.id ? "编辑路由" : "新建路由"}</h2>
          <button className="primary-button compact" onClick={() => onSave(routeDraft)} type="button">
            <Save size={16} aria-hidden="true" />
            <span>保存</span>
          </button>
        </div>

        <div className="form-grid">
          <label>
            <span>名称</span>
            <input value={routeDraft.name} onChange={(event) => updateDraft({ name: event.target.value })} />
          </label>
          <label>
            <span>类型</span>
            <select
              value={routeDraft.type}
              onChange={(event) =>
                updateDraft({
                  type: event.target.value as RouteConfig["type"],
                  upstreamGroupId: event.target.value === "proxy" ? groups[0]?.id : undefined,
                  redirect: event.target.value === "redirect" ? newRedirectConfig() : undefined,
                })
              }
            >
              <option value="proxy">代理</option>
              <option value="redirect">跳转</option>
            </select>
          </label>
          <label>
            <span>优先级</span>
            <input
              type="number"
              value={routeDraft.priority}
              onChange={(event) => updateDraft({ priority: Number(event.target.value) })}
            />
          </label>
          <label>
            <span>主机</span>
            <input value={routeDraft.match.host || ""} onChange={(event) => updateMatch({ host: event.target.value })} />
          </label>
          <label className="wide-field">
            <span>路径</span>
            <input value={routeDraft.match.path} onChange={(event) => updateMatch({ path: event.target.value })} />
          </label>
          <label>
            <span>最大映射字节</span>
            <input
              type="number"
              value={routeDraft.maxResponseBytes || 0}
              onChange={(event) => updateDraft({ maxResponseBytes: Number(event.target.value) })}
            />
          </label>
        </div>

        <div className="method-row">
          {methods.map((method) => (
            <label className="check-chip" key={method}>
              <input
                checked={(routeDraft.match.methods || []).includes(method)}
                onChange={() => toggleMethod(method)}
                type="checkbox"
              />
              <span>{method}</span>
            </label>
          ))}
        </div>

        {routeDraft.type === "proxy" ? (
          <label className="single-field">
            <span>上游分组</span>
            <select
              value={routeDraft.upstreamGroupId || ""}
              onChange={(event) => updateDraft({ upstreamGroupId: event.target.value })}
            >
              <option value="">选择分组</option>
              {groups.map((group) => (
                <option key={group.id} value={group.id}>
                  {group.name || group.id}
                </option>
              ))}
            </select>
          </label>
        ) : (
          <RedirectEditor routeDraft={routeDraft} setRouteDraft={setRouteDraft} />
        )}

        <RulesEditor
          rules={routeDraft.requestRewrite || []}
          title="请求改写"
          onChange={(rules) => updateDraft({ requestRewrite: rules })}
        />
        <MappingRulesEditor
          rules={routeDraft.responseMapping || []}
          title="响应映射"
          onChange={(rules) => updateDraft({ responseMapping: rules })}
        />
      </section>
    </div>
  );
}

function RedirectEditor({
  routeDraft,
  setRouteDraft,
}: {
  routeDraft: RouteConfig;
  setRouteDraft: (route: RouteConfig | ((current: RouteConfig) => RouteConfig)) => void;
}) {
  const redirect = routeDraft.redirect || newRedirectConfig();
  const setRedirect = (patch: Partial<RedirectConfig>) =>
    setRouteDraft((current) => ({
      ...current,
      redirect: { ...(current.redirect || newRedirectConfig()), ...patch },
    }));
  const updateTarget = (index: number, patch: Partial<TargetConfig>) => {
    const targets = redirect.targets.map((target, targetIndex) =>
      targetIndex === index ? { ...target, ...patch } : target,
    );
    setRedirect({ targets });
  };
  return (
    <div className="nested-editor">
      <div className="form-grid">
        <label>
          <span>状态码</span>
          <input
            type="number"
            value={redirect.statusCode}
            onChange={(event) => setRedirect({ statusCode: Number(event.target.value) })}
          />
        </label>
        <label>
          <span>策略</span>
          <select value={redirect.strategy} onChange={(event) => setRedirect({ strategy: event.target.value })}>
            {strategies.map((strategy) => (
              <option key={strategy} value={strategy}>{formatStrategy(strategy)}</option>
            ))}
          </select>
        </label>
      </div>
      <ArrayHeader title="跳转目标" onAdd={() => setRedirect({ targets: [...redirect.targets, newTarget()] })} />
      <div className="rule-stack">
        {redirect.targets.map((target, index) => (
          <div className="target-row" key={index}>
            <input value={target.url} onChange={(event) => updateTarget(index, { url: event.target.value })} />
            <input
              type="number"
              value={target.weight}
              onChange={(event) => updateTarget(index, { weight: Number(event.target.value) })}
            />
            <label className="mini-check">
              <input
                checked={target.enabled}
                onChange={(event) => updateTarget(index, { enabled: event.target.checked })}
                type="checkbox"
              />
              <span>启用</span>
            </label>
            <button
              className="icon-button danger"
              onClick={() => setRedirect({ targets: redirect.targets.filter((_, itemIndex) => itemIndex !== index) })}
              title="移除目标"
              type="button"
            >
              <Trash2 size={16} aria-hidden="true" />
            </button>
          </div>
        ))}
      </div>
    </div>
  );
}

function RulesEditor({
  title,
  rules,
  onChange,
}: {
  title: string;
  rules: RewriteRule[];
  onChange: (rules: RewriteRule[]) => void;
}) {
  const updateRule = (index: number, patch: Partial<RewriteRule>) => {
    onChange(rules.map((rule, ruleIndex) => (ruleIndex === index ? { ...rule, ...patch } : rule)));
  };

  return (
    <section className="inline-section">
      <ArrayHeader title={title} onAdd={() => onChange([...rules, { type: "setHeader", key: "", value: "" }])} />
      <div className="rule-stack">
        {rules.map((rule, index) => (
          <div className="rewrite-row" key={index}>
            <select value={rule.type} onChange={(event) => updateRule(index, { type: event.target.value })}>
              {rewriteTypes.map((type) => (
                <option key={type} value={type}>{formatRewriteType(type)}</option>
              ))}
            </select>
            {rule.type === "rewritePath" ? (
              <>
                <input placeholder="来源路径" value={rule.from || ""} onChange={(event) => updateRule(index, { from: event.target.value })} />
                <input placeholder="目标路径" value={rule.to || ""} onChange={(event) => updateRule(index, { to: event.target.value })} />
              </>
            ) : (
              <>
                <input placeholder="键名" value={rule.key || ""} onChange={(event) => updateRule(index, { key: event.target.value })} />
                <input
                  placeholder="值"
                  value={valueToInput(rule.value)}
                  onChange={(event) => updateRule(index, { value: event.target.value })}
                />
              </>
            )}
            <button
              className="icon-button danger"
              onClick={() => onChange(rules.filter((_, ruleIndex) => ruleIndex !== index))}
              title="删除规则"
              type="button"
            >
              <Trash2 size={16} aria-hidden="true" />
            </button>
          </div>
        ))}
      </div>
    </section>
  );
}

function MappingRulesEditor({
  title,
  rules,
  onChange,
}: {
  title: string;
  rules: MappingRule[];
  onChange: (rules: MappingRule[]) => void;
}) {
  const updateRule = (index: number, patch: Partial<MappingRule>) => {
    onChange(rules.map((rule, ruleIndex) => (ruleIndex === index ? { ...rule, ...patch } : rule)));
  };

  return (
    <section className="inline-section">
      <ArrayHeader title={title} onAdd={() => onChange([...rules, { from: "", to: "$.data.value" }])} />
      <div className="rule-stack">
        {rules.map((rule, index) => (
          <div className="mapping-row" key={index}>
            <input placeholder="来源 JSONPath" value={rule.from || ""} onChange={(event) => updateRule(index, { from: event.target.value })} />
            <input placeholder="目标 JSONPath" value={rule.to} onChange={(event) => updateRule(index, { to: event.target.value })} />
            <input
              placeholder="常量值"
              value={valueToInput(rule.value)}
              onChange={(event) => updateRule(index, { value: event.target.value })}
            />
            <button
              className="icon-button danger"
              onClick={() => onChange(rules.filter((_, ruleIndex) => ruleIndex !== index))}
              title="删除映射"
              type="button"
            >
              <Trash2 size={16} aria-hidden="true" />
            </button>
          </div>
        ))}
      </div>
    </section>
  );
}

function ArrayHeader({ title, onAdd }: { title: string; onAdd: () => void }) {
  return (
    <div className="array-header">
      <h3>{title}</h3>
      <button className="icon-button" onClick={onAdd} title={`新增${title}`} type="button">
        <Plus size={16} aria-hidden="true" />
      </button>
    </div>
  );
}

function UpstreamsPanel({
  groups,
  request,
  reload,
  notify,
}: {
  groups: UpstreamGroup[];
  request: AuthedRequest;
  reload: () => Promise<void>;
  notify: (text: string, type?: NoticeType) => void;
}) {
  const [selectedID, setSelectedID] = useState("");
  const selected = groups.find((group) => group.id === selectedID) || groups[0];
  const [groupDraft, setGroupDraft] = useState<UpstreamGroup>(() => selected || newGroup());
  const [targetDraft, setTargetDraft] = useState<TargetConfig>(() => newTarget());

  useEffect(() => {
    if (!selectedID && groups[0]?.id) {
      setSelectedID(groups[0].id);
    }
  }, [groups, selectedID]);

  useEffect(() => {
    if (selected) {
      setGroupDraft(cloneGroup(selected));
    }
  }, [selected]);

  const saveGroup = async () => {
    const payload = { name: groupDraft.name, strategy: groupDraft.strategy, targets: [] };
    if (groupDraft.id) {
      await request<UpstreamGroup>(`/admin/api/upstream-groups/${encodeURIComponent(groupDraft.id)}`, {
        method: "PUT",
        body: JSON.stringify(payload),
      });
      notify("上游分组已更新");
    } else {
      const created = await request<UpstreamGroup>("/admin/api/upstream-groups", {
        method: "POST",
        body: JSON.stringify(payload),
      });
      setSelectedID(created.id || "");
      notify("上游分组已创建");
    }
    await reload();
  };

  const deleteGroup = async () => {
    if (!groupDraft.id || !window.confirm(`确定删除上游分组「${groupDraft.name || groupDraft.id}」吗？`)) {
      return;
    }
    await request<void>(`/admin/api/upstream-groups/${encodeURIComponent(groupDraft.id)}`, { method: "DELETE" });
    setSelectedID("");
    setGroupDraft(newGroup());
    notify("上游分组已删除");
    await reload();
  };

  const saveTarget = async () => {
    if (!groupDraft.id) {
      notify("请先保存上游分组", "error");
      return;
    }
    const payload = cleanTarget(targetDraft);
    if (targetDraft.id) {
      await request<TargetConfig>(`/admin/api/upstream-targets/${encodeURIComponent(targetDraft.id)}`, {
        method: "PUT",
        body: JSON.stringify(payload),
      });
      notify("目标已更新");
    } else {
      await request<TargetConfig>(`/admin/api/upstream-groups/${encodeURIComponent(groupDraft.id)}/targets`, {
        method: "POST",
        body: JSON.stringify(payload),
      });
      notify("目标已创建");
    }
    setTargetDraft(newTarget());
    await reload();
  };

  const deleteTarget = async (target: TargetConfig) => {
    if (!target.id || !window.confirm(`确定删除目标「${target.url}」吗？`)) {
      return;
    }
    await request<void>(`/admin/api/upstream-targets/${encodeURIComponent(target.id)}`, { method: "DELETE" });
    setTargetDraft(newTarget());
    notify("目标已删除");
    await reload();
  };

  return (
    <div className="split-layout upstream-layout">
      <section className="surface">
        <div className="section-title">
          <h2>分组</h2>
          <button className="primary-button compact" onClick={() => setGroupDraft(newGroup())} type="button">
            <Plus size={16} aria-hidden="true" />
            <span>分组</span>
          </button>
        </div>
        <div className="compact-list">
          {groups.map((group) => (
            <button
              className={groupDraft.id === group.id ? "select-row active" : "select-row"}
              key={group.id || group.name}
              onClick={() => {
                setSelectedID(group.id || "");
                setGroupDraft(cloneGroup(group));
                setTargetDraft(newTarget());
              }}
              type="button"
            >
              <div>
                <strong>{group.name || group.id}</strong>
                <span>{formatStrategy(group.strategy)}</span>
              </div>
              <span className="pill">{group.targets.length} 个目标</span>
            </button>
          ))}
          {groups.length === 0 ? <EmptyLine text="暂无上游分组" /> : null}
        </div>
      </section>

      <section className="surface editor">
        <div className="section-title">
          <h2>{groupDraft.id ? "编辑分组" : "新建分组"}</h2>
          <div className="icon-row">
            {groupDraft.id ? (
              <button className="icon-button danger" onClick={() => void deleteGroup()} title="删除分组" type="button">
                <Trash2 size={16} aria-hidden="true" />
              </button>
            ) : null}
            <button className="primary-button compact" onClick={() => void saveGroup()} type="button">
              <Save size={16} aria-hidden="true" />
              <span>保存</span>
            </button>
          </div>
        </div>

        <div className="form-grid">
          <label>
            <span>名称</span>
            <input
              value={groupDraft.name || ""}
              onChange={(event) => setGroupDraft((current) => ({ ...current, name: event.target.value }))}
            />
          </label>
          <label>
            <span>策略</span>
            <select
              value={groupDraft.strategy}
              onChange={(event) => setGroupDraft((current) => ({ ...current, strategy: event.target.value }))}
            >
              {strategies.map((strategy) => (
                <option key={strategy} value={strategy}>{formatStrategy(strategy)}</option>
              ))}
            </select>
          </label>
        </div>

        <div className="section-title small-title">
          <h2>目标</h2>
          <button className="primary-button compact" onClick={() => setTargetDraft(newTarget())} type="button">
            <Plus size={16} aria-hidden="true" />
            <span>目标</span>
          </button>
        </div>
        <div className="table-wrap">
          <table>
            <thead>
              <tr>
                <th>URL</th>
                <th>权重</th>
                <th>健康状态</th>
                <th>启用状态</th>
                <th aria-label="操作" />
              </tr>
            </thead>
            <tbody>
              {(selected?.targets || []).map((target) => (
                <tr key={target.id || target.url}>
                  <td>
                    <code>{target.url}</code>
                  </td>
                  <td>{target.weight}</td>
                  <td>{formatHealthStatus(target.healthStatus)}</td>
                  <td>
                    <span className={`pill ${target.enabled ? "ok" : "muted"}`}>{formatEnabled(target.enabled)}</span>
                  </td>
                  <td>
                    <div className="icon-row">
                      <button className="icon-button" onClick={() => setTargetDraft({ ...target })} title="编辑目标" type="button">
                        <Pencil size={16} aria-hidden="true" />
                      </button>
                      <button
                        className="icon-button danger"
                        onClick={() => void deleteTarget(target)}
                        title="删除目标"
                        type="button"
                      >
                        <Trash2 size={16} aria-hidden="true" />
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
              {selected?.targets.length ? null : (
                <tr>
                  <td colSpan={5}>
                    <EmptyLine text="该分组暂无目标" />
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>

        <div className="target-editor">
          <h3>{targetDraft.id ? "编辑目标" : "新建目标"}</h3>
          <div className="form-grid">
            <label className="wide-field">
              <span>URL</span>
              <input value={targetDraft.url} onChange={(event) => setTargetDraft((current) => ({ ...current, url: event.target.value }))} />
            </label>
            <label>
              <span>权重</span>
              <input
                type="number"
                value={targetDraft.weight}
                onChange={(event) => setTargetDraft((current) => ({ ...current, weight: Number(event.target.value) }))}
              />
            </label>
            <label>
              <span>健康状态</span>
              <select
                value={targetDraft.healthStatus || "unknown"}
                onChange={(event) => setTargetDraft((current) => ({ ...current, healthStatus: event.target.value }))}
              >
                <option value="unknown">未知</option>
                <option value="healthy">健康</option>
                <option value="unhealthy">不健康</option>
              </select>
            </label>
          </div>
          <label className="check-chip standalone">
            <input
              checked={targetDraft.enabled}
              onChange={(event) => setTargetDraft((current) => ({ ...current, enabled: event.target.checked }))}
              type="checkbox"
            />
            <span>启用</span>
          </label>
          <button className="primary-button compact" onClick={() => void saveTarget()} type="button">
            <Save size={16} aria-hidden="true" />
            <span>保存目标</span>
          </button>
        </div>
      </section>
    </div>
  );
}

function MappingPanel({
  request,
  notify,
}: {
  request: AuthedRequest;
  notify: (text: string, type?: NoticeType) => void;
}) {
  const [sourceText, setSourceText] = useState(sampleSource);
  const [rules, setRules] = useState<MappingRule[]>([
    { from: "$.result.username", to: "$.data.name" },
    { from: "$.result.userId", to: "$.data.id" },
    { value: "true", to: "$.success" },
  ]);
  const [result, setResult] = useState("");

  const preview = async () => {
    try {
      const source = JSON.parse(sourceText);
      const response = await request<{ result: unknown }>("/admin/api/debug/mapping", {
        method: "POST",
        body: JSON.stringify({ source, rules: cleanMappingRules(rules) }),
      });
      setResult(JSON.stringify(response.result, null, 2));
      notify("映射预览已生成");
    } catch (error) {
      notify(errorMessage(error), "error");
    }
  };

  return (
    <div className="mapping-workbench">
      <section className="surface">
        <div className="section-title">
          <h2>源 JSON</h2>
          <Braces size={18} aria-hidden="true" />
        </div>
        <textarea value={sourceText} onChange={(event) => setSourceText(event.target.value)} spellCheck={false} />
      </section>
      <section className="surface">
        <div className="section-title">
          <h2>规则</h2>
          <button className="primary-button compact" onClick={() => void preview()} type="button">
            <Play size={16} aria-hidden="true" />
            <span>预览</span>
          </button>
        </div>
        <MappingRulesEditor rules={rules} title="预览映射" onChange={setRules} />
      </section>
      <section className="surface">
        <div className="section-title">
          <h2>结果 JSON</h2>
          <Database size={18} aria-hidden="true" />
        </div>
        <textarea readOnly value={result} spellCheck={false} />
      </section>
    </div>
  );
}

function DebugPanel({
  request,
  notify,
}: {
  request: AuthedRequest;
  notify: (text: string, type?: NoticeType) => void;
}) {
  const [url, setURL] = useState("http://localhost:8080/api/users");
  const [method, setMethod] = useState("GET");
  const [headersText, setHeadersText] = useState(sampleHeaders);
  const [body, setBody] = useState("");
  const [result, setResult] = useState<DebugResult | null>(null);

  const send = async () => {
    try {
      const headers = headersText.trim() ? JSON.parse(headersText) : {};
      const response = await request<DebugResult>("/admin/api/debug/request", {
        method: "POST",
        body: JSON.stringify({ url, method, headers, body }),
      });
      setResult(response);
      notify("请求已完成");
    } catch (error) {
      notify(errorMessage(error), "error");
    }
  };

  return (
    <div className="debug-grid">
      <section className="surface">
        <div className="section-title">
          <h2>请求</h2>
          <button className="primary-button compact" onClick={() => void send()} type="button">
            <Play size={16} aria-hidden="true" />
            <span>发送</span>
          </button>
        </div>
        <div className="form-grid debug-form">
          <label>
            <span>方法</span>
            <select value={method} onChange={(event) => setMethod(event.target.value)}>
              {methods.map((item) => (
                <option key={item}>{item}</option>
              ))}
            </select>
          </label>
          <label className="wide-field">
            <span>URL</span>
            <input value={url} onChange={(event) => setURL(event.target.value)} />
          </label>
        </div>
        <label className="single-field">
          <span>请求头 JSON</span>
          <textarea value={headersText} onChange={(event) => setHeadersText(event.target.value)} spellCheck={false} />
        </label>
        <label className="single-field">
          <span>请求体</span>
          <textarea value={body} onChange={(event) => setBody(event.target.value)} spellCheck={false} />
        </label>
      </section>

      <section className="surface">
        <div className="section-title">
          <h2>响应</h2>
          {result ? (
            <span className={`pill ${result.statusCode < 400 ? "ok" : "warn"}`}>
              {result.statusCode} / {result.durationMs}ms
            </span>
          ) : null}
        </div>
        <pre className="response-box">
          {result
            ? JSON.stringify(
                {
                  statusCode: result.statusCode,
                  durationMs: result.durationMs,
                  truncated: result.truncated,
                  headers: result.headers,
                  body: tryFormatJSON(result.body),
                },
                null,
                2,
              )
            : "暂无响应"}
        </pre>
      </section>
    </div>
  );
}

function LogsPanel({
  request,
  notify,
}: {
  request: AuthedRequest;
  notify: (text: string, type?: NoticeType) => void;
}) {
  const [logs, setLogs] = useState<RequestLog[]>([]);
  const [limit, setLimit] = useState(100);
  const [loading, setLoading] = useState(false);

  const loadLogs = useCallback(async () => {
    setLoading(true);
    try {
      const response = await request<RequestLog[]>(`/admin/api/request-logs?limit=${limit}`);
      setLogs(response);
    } catch (error) {
      notify(errorMessage(error), "error");
    } finally {
      setLoading(false);
    }
  }, [limit, notify, request]);

  useEffect(() => {
    void loadLogs();
  }, [loadLogs]);

  const successCount = logs.filter((log) => log.statusCode < 400).length;
  const errorCount = logs.length - successCount;
  const averageDuration = logs.length
    ? Math.round(logs.reduce((total, log) => total + log.durationMs, 0) / logs.length)
    : 0;

  return (
    <div className="logs-layout">
      <div className="logs-metrics">
        <Metric label="请求数" value={logs.length} detail={`最近 ${limit} 条`} icon={FileText} />
        <Metric label="成功请求" value={successCount} detail={`${errorCount} 条错误`} icon={CheckCircle2} />
        <Metric label="平均延迟" value={averageDuration} detail="毫秒" icon={Gauge} />
      </div>

      <section className="surface">
        <div className="section-title">
          <h2>请求日志</h2>
          <div className="toolbar-row">
            <label className="inline-select">
              <span>数量</span>
              <select value={limit} onChange={(event) => setLimit(Number(event.target.value))}>
                <option value={50}>50</option>
                <option value={100}>100</option>
                <option value={250}>250</option>
                <option value={500}>500</option>
              </select>
            </label>
            <button className="primary-button compact" onClick={() => void loadLogs()} type="button">
              <RefreshCw size={16} aria-hidden="true" className={loading ? "spin" : ""} />
              <span>刷新</span>
            </button>
          </div>
        </div>
        <div className="table-wrap">
          <table className="logs-table">
            <thead>
              <tr>
                <th>时间</th>
                <th>方法</th>
                <th>路径</th>
                <th>路由</th>
                <th>上游</th>
                <th>状态</th>
                <th>延迟</th>
                <th>客户端</th>
                <th>错误</th>
              </tr>
            </thead>
            <tbody>
              {logs.map((log) => (
                <tr key={log.id}>
                  <td>{formatDateTime(log.createdAt)}</td>
                  <td>
                    <span className="method-pill">{log.method}</span>
                  </td>
                  <td>
                    <code>{log.path}</code>
                    <span className="subtext">{log.requestId}</span>
                  </td>
                  <td>{log.routeId || "-"}</td>
                  <td>
                    {log.upstreamUrl ? <code>{log.upstreamUrl}</code> : <span className="subtext">-</span>}
                  </td>
                  <td>
                    <span className={`pill ${statusClass(log.statusCode)}`}>{log.statusCode}</span>
                  </td>
                  <td>{log.durationMs}ms</td>
                  <td>{log.clientIp || "-"}</td>
                  <td>{log.error || "-"}</td>
                </tr>
              ))}
              {logs.length === 0 ? (
                <tr>
                  <td colSpan={9}>
                    <EmptyLine text={loading ? "正在加载请求日志" : "暂无请求日志"} />
                  </td>
                </tr>
              ) : null}
            </tbody>
          </table>
        </div>
      </section>
    </div>
  );
}

function AuditPanel({
  request,
  notify,
}: {
  request: AuthedRequest;
  notify: (text: string, type?: NoticeType) => void;
}) {
  const [logs, setLogs] = useState<AuditLog[]>([]);
  const [limit, setLimit] = useState(100);
  const [loading, setLoading] = useState(false);

  const loadLogs = useCallback(async () => {
    setLoading(true);
    try {
      const response = await request<AuditLog[]>(`/admin/api/audit-logs?limit=${limit}`);
      setLogs(response);
    } catch (error) {
      notify(errorMessage(error), "error");
    } finally {
      setLoading(false);
    }
  }, [limit, notify, request]);

  useEffect(() => {
    void loadLogs();
  }, [loadLogs]);

  const resourceTypes = new Set(logs.map((log) => log.resourceType));
  const actors = new Set(logs.map((log) => String(log.detail?.username || log.adminUserId || "-")));

  return (
    <div className="logs-layout">
      <div className="logs-metrics">
        <Metric label="审计事件" value={logs.length} detail={`最近 ${limit} 条`} icon={ShieldCheck} />
        <Metric label="操作者" value={actors.size} detail="后台用户" icon={Shield} />
        <Metric label="资源类型" value={resourceTypes.size} detail="资源分类" icon={Database} />
      </div>

      <section className="surface">
        <div className="section-title">
          <h2>审计日志</h2>
          <div className="toolbar-row">
            <label className="inline-select">
              <span>数量</span>
              <select value={limit} onChange={(event) => setLimit(Number(event.target.value))}>
                <option value={50}>50</option>
                <option value={100}>100</option>
                <option value={250}>250</option>
                <option value={500}>500</option>
              </select>
            </label>
            <button className="primary-button compact" onClick={() => void loadLogs()} type="button">
              <RefreshCw size={16} aria-hidden="true" className={loading ? "spin" : ""} />
              <span>刷新</span>
            </button>
          </div>
        </div>
        <div className="table-wrap">
          <table className="audit-table">
            <thead>
              <tr>
                <th>时间</th>
                <th>操作者</th>
                <th>动作</th>
                <th>资源</th>
                <th>资源 ID</th>
                <th>详情</th>
              </tr>
            </thead>
            <tbody>
              {logs.map((log) => (
                <tr key={log.id}>
                  <td>{formatDateTime(log.createdAt)}</td>
                  <td>
                    <strong>{String(log.detail?.username || "-")}</strong>
                    <span className="subtext">{log.adminUserId || "-"}</span>
                  </td>
                  <td>
                    <span className="method-pill">{formatAuditAction(log.action)}</span>
                  </td>
                  <td>{formatResourceType(log.resourceType)}</td>
                  <td>
                    <code>{log.resourceId || "-"}</code>
                  </td>
                  <td>
                    <pre className="inline-json">{formatCompactJSON(cleanAuditDetail(log.detail))}</pre>
                  </td>
                </tr>
              ))}
              {logs.length === 0 ? (
                <tr>
                  <td colSpan={6}>
                    <EmptyLine text={loading ? "正在加载审计日志" : "暂无审计日志"} />
                  </td>
                </tr>
              ) : null}
            </tbody>
          </table>
        </div>
      </section>
    </div>
  );
}

function AccountPanel({
  request,
  notify,
  user,
  onUpdated,
}: {
  request: AuthedRequest;
  notify: (text: string, type?: NoticeType) => void;
  user: AdminUser | null;
  onUpdated: (user: AdminUser) => void;
}) {
  const [username, setUsername] = useState(user?.username || "");
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [saving, setSaving] = useState(false);

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    const trimmedUsername = username.trim();
    if (!trimmedUsername) {
      notify("用户名不能为空", "error");
      return;
    }
    if (!currentPassword) {
      notify("请输入当前密码", "error");
      return;
    }
    if (newPassword) {
      if (newPassword.length < 8) {
        notify("新密码至少需要 8 个字符", "error");
        return;
      }
      if (newPassword !== confirmPassword) {
        notify("两次输入的新密码不一致", "error");
        return;
      }
    }

    setSaving(true);
    try {
      const updated = await request<AdminUser>("/admin/api/auth/me", {
        method: "PUT",
        body: JSON.stringify({
          username: trimmedUsername,
          currentPassword,
          newPassword: newPassword || undefined,
        }),
      });
      onUpdated(updated);
      setCurrentPassword("");
      setNewPassword("");
      setConfirmPassword("");
      notify(newPassword ? "账户与密码已更新" : "账户已更新");
    } catch (error) {
      notify(errorMessage(error), "error");
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="account-layout">
      <section className="surface">
        <div className="section-title">
          <h2>账户信息</h2>
          <KeyRound size={18} aria-hidden="true" />
        </div>
        <form className="account-form" onSubmit={submit}>
          <label>
            <span>用户名</span>
            <input autoComplete="username" value={username} onChange={(event) => setUsername(event.target.value)} />
          </label>
          <label>
            <span>当前密码</span>
            <input
              autoComplete="current-password"
              type="password"
              value={currentPassword}
              onChange={(event) => setCurrentPassword(event.target.value)}
              placeholder="验证身份所需"
            />
          </label>

          <div className="account-divider">
            <span>修改密码（可选）</span>
          </div>

          <label>
            <span>新密码</span>
            <input
              autoComplete="new-password"
              type="password"
              value={newPassword}
              onChange={(event) => setNewPassword(event.target.value)}
              placeholder="留空表示不修改，至少 8 位"
            />
          </label>
          <label>
            <span>确认新密码</span>
            <input
              autoComplete="new-password"
              type="password"
              value={confirmPassword}
              onChange={(event) => setConfirmPassword(event.target.value)}
              placeholder="再次输入新密码"
            />
          </label>

          <button className="primary-button" disabled={saving} type="submit">
            <Save size={16} aria-hidden="true" />
            <span>{saving ? "保存中" : "保存账户"}</span>
          </button>
        </form>
      </section>
    </div>
  );
}

function EmptyLine({ text }: { text: string }) {
  return <div className="empty-line">{text}</div>;
}

async function apiFetch<T>(apiBase: string, token: string, path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers);
  if (!headers.has("Content-Type") && init.body) {
    headers.set("Content-Type", "application/json");
  }
  if (token) {
    headers.set("Authorization", `Bearer ${token}`);
  }
  const response = await fetch(`${apiBase}${path}`, { ...init, headers });
  const text = await response.text();
  const data = text ? safeJSON(text) : undefined;
  if (!response.ok) {
    throw new Error(readAPIError(data, response.statusText));
  }
  return data as T;
}

function cleanRoute(route: RouteConfig): RouteConfig {
  const cleaned: RouteConfig = {
    name: route.name.trim() || "unnamed-route",
    enabled: route.enabled,
    priority: Number(route.priority) || 0,
    type: route.type,
    match: {
      host: route.match.host?.trim() || undefined,
      path: route.match.path.trim() || "/",
      methods: route.match.methods || [],
    },
    requestRewrite: cleanRewriteRules(route.requestRewrite || []),
    responseMapping: cleanMappingRules(route.responseMapping || []),
    maxResponseBytes: Number(route.maxResponseBytes) || 0,
  };
  if (route.id) {
    cleaned.id = route.id;
  }
  if (route.type === "proxy") {
    cleaned.upstreamGroupId = route.upstreamGroupId;
  } else {
    cleaned.redirect = {
      statusCode: route.redirect?.statusCode || 302,
      strategy: route.redirect?.strategy || "round-robin",
      targets: (route.redirect?.targets || []).map(cleanTarget),
    };
  }
  return cleaned;
}

function cleanRewriteRules(rules: RewriteRule[]): RewriteRule[] {
  return rules
    .map((rule) => {
      const cleaned: RewriteRule = { type: rule.type || "setHeader" };
      if (rule.type === "rewritePath") {
        cleaned.from = rule.from || "";
        cleaned.to = rule.to || "";
      } else {
        cleaned.key = rule.key || "";
        const value = parseFieldValue(rule.value);
        if (value !== undefined) {
          cleaned.value = value;
        }
      }
      return cleaned;
    })
    .filter((rule) => rule.type === "rewritePath" || rule.key);
}

function cleanMappingRules(rules: MappingRule[]): MappingRule[] {
  return rules
    .map((rule) => {
      const cleaned: MappingRule = { to: rule.to || "" };
      if (rule.from?.trim()) {
        cleaned.from = rule.from.trim();
      }
      const value = parseFieldValue(rule.value);
      if (value !== undefined) {
        cleaned.value = value;
      }
      return cleaned;
    })
    .filter((rule) => rule.to && (rule.from || rule.value !== undefined));
}

function cleanTarget(target: TargetConfig): TargetConfig {
  return {
    id: target.id,
    groupId: target.groupId,
    url: target.url.trim(),
    weight: Number(target.weight) || 1,
    enabled: Boolean(target.enabled),
    healthStatus: target.healthStatus || "unknown",
  };
}

function parseFieldValue(value: unknown): unknown {
  if (typeof value !== "string") {
    return value;
  }
  const trimmed = value.trim();
  if (trimmed === "") {
    return undefined;
  }
  try {
    return JSON.parse(trimmed);
  } catch {
    return value;
  }
}

function valueToInput(value: unknown): string {
  if (value === undefined) {
    return "";
  }
  if (typeof value === "string") {
    return value;
  }
  return JSON.stringify(value);
}

function readStoredUser(): AdminUser | null {
  const text = localStorage.getItem(userKey);
  if (!text) {
    return null;
  }
  try {
    return JSON.parse(text) as AdminUser;
  } catch {
    return null;
  }
}

function safeJSON(text: string): unknown {
  try {
    return JSON.parse(text);
  } catch {
    return text;
  }
}

function readAPIError(data: unknown, fallback: string): string {
  if (data && typeof data === "object" && "error" in data) {
    return translateAPIError(String((data as { error?: unknown }).error));
  }
  return translateAPIError(fallback || "请求失败");
}

function normalizeBaseURL(value: string): string {
  return value.trim().replace(/\/+$/, "");
}

function errorMessage(error: unknown): string {
  return translateAPIError(error instanceof Error ? error.message : String(error));
}

function translateAPIError(message: string): string {
  const normalized = message.trim();
  const messages: Record<string, string> = {
    "missing bearer token": "登录已失效，请重新登录",
    "invalid bearer token": "登录已失效，请重新登录",
    "invalid username or password": "用户名或密码错误",
    "current password is incorrect": "当前密码不正确",
    "username already taken": "用户名已被占用",
    "username is required": "用户名不能为空",
    "new password must be at least 8 characters": "新密码至少需要 8 个字符",
    "account not found": "账户不存在",
    "method not allowed": "请求方法不允许",
    "not found": "资源不存在",
    "invalid json body": "请求内容不是有效 JSON",
    "Request failed": "请求失败",
    "Failed to fetch": "请求失败，请检查管理 API 地址",
  };
  return messages[normalized] || normalized || "请求失败";
}

function newRouteDraft(groups: UpstreamGroup[] = []): RouteConfig {
  return {
    name: "新路由",
    enabled: true,
    priority: 100,
    type: "proxy",
    match: { path: "/api/**", methods: ["GET"] },
    upstreamGroupId: groups[0]?.id,
    requestRewrite: [],
    responseMapping: [],
    maxResponseBytes: 1048576,
  };
}

function newRedirectConfig(): RedirectConfig {
  return {
    statusCode: 302,
    strategy: "round-robin",
    targets: [newTarget("https://example.com")],
  };
}

function newTarget(url = "http://127.0.0.1:9001"): TargetConfig {
  return { url, weight: 1, enabled: true, healthStatus: "unknown" };
}

function newGroup(): UpstreamGroup {
  return { name: "新分组", strategy: "round-robin", targets: [] };
}

function cloneRoute(route: RouteConfig): RouteConfig {
  return JSON.parse(JSON.stringify(route)) as RouteConfig;
}

function cloneGroup(group: UpstreamGroup): UpstreamGroup {
  return JSON.parse(JSON.stringify(group)) as UpstreamGroup;
}

function tryFormatJSON(text: string): unknown {
  const parsed = safeJSON(text);
  return typeof parsed === "string" ? text : parsed;
}

function formatDateTime(value: string): string {
  if (!value) {
    return "-";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return date.toLocaleString();
}

function statusClass(statusCode: number): string {
  if (statusCode >= 500) {
    return "bad";
  }
  if (statusCode >= 400) {
    return "warn";
  }
  return "ok";
}

function cleanAuditDetail(detail: Record<string, unknown>): Record<string, unknown> {
  const cleaned = { ...detail };
  delete cleaned.username;
  delete cleaned.role;
  return cleaned;
}

function formatCompactJSON(value: unknown): string {
  return JSON.stringify(value, null, 2);
}

function formatRouteType(type: string): string {
  switch (type) {
    case "proxy":
      return "代理";
    case "redirect":
      return "跳转";
    default:
      return type || "-";
  }
}

function formatStrategy(strategy: string): string {
  switch (strategy) {
    case "round-robin":
      return "轮询";
    case "weighted-round-robin":
    case "weighted":
      return "加权轮询";
    case "random":
      return "随机";
    default:
      return strategy || "-";
  }
}

function formatRewriteType(type: string): string {
  switch (type) {
    case "setHeader":
      return "设置 Header";
    case "setQuery":
      return "设置 Query";
    case "rewritePath":
      return "改写路径";
    case "setJsonBody":
      return "设置 JSON Body";
    default:
      return type || "-";
  }
}

function formatHealthStatus(status?: string): string {
  switch (status) {
    case "healthy":
      return "健康";
    case "unhealthy":
      return "不健康";
    default:
      return "未知";
  }
}

function formatEnabled(enabled: boolean): string {
  return enabled ? "启用" : "停用";
}

function formatAuditAction(action: string): string {
  switch (action) {
    case "create":
      return "创建";
    case "update":
      return "更新";
    case "delete":
      return "删除";
    case "login":
      return "登录";
    default:
      return action || "-";
  }
}

function formatResourceType(resourceType: string): string {
  switch (resourceType) {
    case "route":
      return "路由";
    case "upstream_group":
      return "上游分组";
    case "upstream_target":
      return "上游目标";
    case "admin_user":
      return "管理员账户";
    case "auth":
      return "认证";
    default:
      return resourceType || "-";
  }
}

function tabTitle(tab: TabID): string {
  const titles: Record<TabID, string> = {
    dashboard: "仪表盘",
    routes: "路由管理",
    upstreams: "上游服务管理",
    mapping: "响应映射设计器",
    debug: "调试控制台",
    logs: "请求日志",
    audit: "审计日志",
    account: "账户设置",
  };
  return titles[tab];
}

export default App;
