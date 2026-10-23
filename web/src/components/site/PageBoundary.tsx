import { Component, type ReactNode } from 'react'
import { siteText } from '@/lib/site-copy'
import type { Lang } from '@/lib/i18n'
// Native documents keep one page identity. A failed deferred module offers a
// deliberate document reload; recovering code must never replay a commerce intent.
export default class PageBoundary extends Component<{ children: ReactNode; lang: Lang }, { failed: boolean }> {
  state = { failed: false }
  static getDerivedStateFromError() { return { failed: true } }
  render() {
    if (!this.state.failed) return this.props.children
    const { lang } = this.props
    return <section className="empty-state stack" role="alert"><h1>{siteText(lang, 'pageUnavailable')}</h1><p>{siteText(lang, 'pageReloadHelp')}</p><button type="button" className="site-button site-button-outline" onClick={() => window.location.reload()}>{siteText(lang, 'reloadPage')}</button></section>
  }
}
