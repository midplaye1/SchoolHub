export async function api(path, options = {}) {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), 12000);
  try {
    const response = await fetch(`/api${path}`, {
      ...options, signal: options.signal || controller.signal,
      headers: { 'Content-Type': 'application/json', ...options.headers },
      body: options.body === undefined ? undefined : JSON.stringify(options.body)
    });
    if (response.status === 204) return null;
    let data;
    try { data = await response.json(); } catch { throw new Error('Сервер вернул некорректный ответ'); }
    if (!response.ok) throw new Error(data.error || 'Не удалось выполнить запрос');
    return data;
  } catch (error) {
    if (error.name === 'AbortError') throw new Error('Время ожидания истекло. Попробуйте ещё раз.');
    if (error instanceof TypeError) throw new Error('Нет связи с сервером. Проверьте подключение.');
    throw error;
  } finally { clearTimeout(timer); }
}
export function query(values) {
  return new URLSearchParams(Object.entries(values).filter(([, value]) => value !== '' && value !== undefined && value !== null)).toString();
}
