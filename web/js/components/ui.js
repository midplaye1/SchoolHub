export const esc = value => String(value ?? '').replace(/[&<>"']/g, c => ({ '&':'&amp;', '<':'&lt;', '>':'&gt;', '"':'&quot;', "'":'&#39;' }[c]));
export const icon = (name, cls = '') => `<i data-lucide="${esc(name)}" class="${cls}"></i>`;
export function icons() { window.lucide?.createIcons(); }
export const number = value => new Intl.NumberFormat('ru-RU').format(value || 0);
export const date = value => value ? new Date(value).toLocaleDateString('ru-RU', { day:'2-digit', month:'short', year:'numeric' }) : '—';
export function localDay(offset = 0) { const d = new Date(); d.setDate(d.getDate() + offset); return `${d.getFullYear()}-${String(d.getMonth()+1).padStart(2,'0')}-${String(d.getDate()).padStart(2,'0')}`; }
export function toast(message, error = false) {
 const node = document.createElement('div'); node.className = `toast ${error ? 'error' : ''}`;
 node.innerHTML = `${icon(error ? 'circle-alert' : 'circle-check')}<span>${esc(message)}</span><button class="icon-button" aria-label="Закрыть">${icon('x')}</button>`;
 node.querySelector('button').onclick = () => node.remove(); document.getElementById('toasts').append(node); icons(); setTimeout(() => node.remove(), 6000);
}
export function empty(title = 'Пока ничего нет', description = 'Добавьте первую запись, чтобы начать работу.') { return `<div class="empty">${icon('inbox')}<strong>${esc(title)}</strong><p>${esc(description)}</p></div>`; }
export function pagination(data) {
 const pages = Math.max(1, Math.ceil(data.total / data.limit));
 return `<div class="pagination"><span>${data.total ? `${(data.page-1)*data.limit+1}–${Math.min(data.page*data.limit,data.total)} из ${number(data.total)}` : 'Нет записей'}</span><div><button class="icon-button" data-pagination="${data.page-1}" ${data.page<=1?'disabled':''} aria-label="Предыдущая страница">${icon('chevron-left')}</button><span>Страница ${data.page} из ${pages}</span><button class="icon-button" data-pagination="${data.page+1}" ${data.page>=pages?'disabled':''} aria-label="Следующая страница">${icon('chevron-right')}</button></div></div>`;
}
export function bookCover(title, id, small = false) { return `<span class="book-cover cover-${Number(id)%6} ${small?'mini':''}" aria-hidden="true"><span>${esc(title.slice(0,24))}</span><span class="cover-lines"></span></span>`; }
export function status(loan) { return loan.returned_at ? '<span class="badge neutral">Возвращена</span>' : loan.overdue ? '<span class="badge danger"><span></span>Просрочена</span>' : '<span class="badge success"><span></span>На руках</span>'; }
