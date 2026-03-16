(function () {
  const AUTH_TOKEN_KEY = 'authToken';
  const REDIRECT_PARAM = 'redirect';

  function getToken() {
    return localStorage.getItem(AUTH_TOKEN_KEY) || '';
  }

  function setToken(token) {
    if (token) {
      localStorage.setItem(AUTH_TOKEN_KEY, token);
    } else {
      localStorage.removeItem(AUTH_TOKEN_KEY);
    }
  }

  function clearAuth() {
    localStorage.removeItem(AUTH_TOKEN_KEY);
  }

  function sanitizeRedirect(target) {
    if (!target || typeof target !== 'string') return '/';
    if (!target.startsWith('/')) return '/';
    if (target.startsWith('//')) return '/';
    return target;
  }

  function currentPath() {
    return `${window.location.pathname}${window.location.search}${window.location.hash}`;
  }

  function buildLoginUrl(target) {
    const redirect = sanitizeRedirect(target || currentPath());
    const params = new URLSearchParams();
    if (redirect && redirect !== '/login.html') {
      params.set(REDIRECT_PARAM, redirect);
    }
    const query = params.toString();
    return `/login.html${query ? `?${query}` : ''}`;
  }

  function redirectToLogin(target) {
    window.location.replace(buildLoginUrl(target));
  }

  function getPostLoginRedirect() {
    const redirect = new URLSearchParams(window.location.search).get(REDIRECT_PARAM);
    return sanitizeRedirect(redirect || '/');
  }

  async function fetchCurrentUser(apiBase) {
    const token = getToken();
    if (!token) return null;

    const res = await fetch(`${apiBase}/auth/me`, {
      headers: {
        Authorization: `Bearer ${token}`,
      },
    });

    if (!res.ok) {
      clearAuth();
      return null;
    }

    return res.json();
  }

  window.TixAuth = {
    buildLoginUrl,
    clearAuth,
    fetchCurrentUser,
    getPostLoginRedirect,
    getToken,
    redirectToLogin,
    setToken,
  };
})();
