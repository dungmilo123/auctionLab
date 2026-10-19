import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Field, LotCard, LotImage, PageHeading, StatusExamples, UnavailableState } from '@/components/site/Presentation'
import { HERO_IMAGE, PREVIEW_LOTS } from '@/fixtures/site'
import { useSiteCopy } from '@/lib/site-copy'
import type { Page } from '@/lib/route'

function LotGrid() {
  const { t } = useSiteCopy()
  return <div className="lot-grid">{PREVIEW_LOTS.map(lot => <LotCard key={lot.href}
    {...lot} title={t(lot.title)} metadata={t(lot.metadata)} />)}</div>
}

export default function StorefrontPage({ page }: { page: Page }) {
  const { t } = useSiteCopy()
  if (page === 'notFound') return <div className="not-found stack">
    <PageHeading title={t('notFound')} intro={t('notFoundNote')} />
    <Button asChild><a href="/">{t('homeLink')}</a></Button>
  </div>
  if (page === 'home') return <>
    <section className="hero">
      <div className="stack">
        <p className="eyebrow">AuctionLab · {t('concept')}</p>
        <h1>{t('tagline')}</h1><p className="lede">{t('intro')}</p>
        <div className="actions"><Button asChild><a href="/lots">{t('browseLots')}</a></Button>
          <Button variant="link" asChild><a href="/auctions">{t('browseAuctions')}</a></Button></div>
      </div>
      <div className="hero-art stack-small"><LotImage src={HERO_IMAGE} alt={t('imageAlt', { object: t('camera') })} />
        <p className="muted">{t('illustration')} · {t('camera')}</p></div>
    </section>
    <section><div className="section-heading"><div className="stack-small"><p className="eyebrow">{t('illustration')}</p>
      <h2>{t('collection')}</h2><p className="muted">{t('collectionNote')}</p></div><a href="/lots">{t('viewAll')} →</a></div><LotGrid /></section>
  </>
  if (page === 'auctions' || page === 'auctionDetail') return <div className="stack">
    <PageHeading title={t(page === 'auctions' ? 'auctions' : 'auctionDetail')} intro={t('auctionIntro')} />
    <Card><CardContent className="stack">
      <p className="eyebrow">{t('illustration')}</p><h2>{t('auctionTitle')}</h2>
      <p className="muted numeric">{t('schedule')}: {t('unavailable')}</p>
      <StatusExamples />
      {page === 'auctions' ? <Button variant="outline" asChild><a href="/auctions/analog-memories">{t('openAuction')}</a></Button>
        : <Button variant="outline" asChild><a href="/lots">{t('browseLots')}</a></Button>}
    </CardContent></Card>
    {page === 'auctionDetail' && <LotGrid />}
  </div>
  if (page === 'lotDetail') return <div className="stack">
    <PageHeading title={t('lotDetail')} intro={t('detailIntro')} />
    <div className="detail-grid">
      <LotImage src={HERO_IMAGE} alt={t('imageAlt', { object: t('camera') })} />
      <div className="stack"><p className="eyebrow">{t('illustration')}</p><h2>{t('camera')}</h2>
        <p className="muted">{t('cameraMeta')}</p>
        <dl className="metadata numeric"><div><dt>{t('price')}</dt><dd>{t('unavailable')}</dd></div>
          <div><dt>{t('schedule')}</dt><dd>{t('unavailable')}</dd></div></dl>
        <Button type="button" disabled aria-describedby="bid-unavailable">{t('bid')}</Button>
        <p id="bid-unavailable" className="muted">{t('disabledNote')}</p>
      </div>
    </div>
  </div>
  return <div className="stack">
    <PageHeading title={t('lots')} intro={t('lotsIntro')} />
    <LotGrid />
    <Card><CardContent className="stack"><h2>{t('fieldTitle')}</h2>
      <div className="field-grid">
        <Field id="search-example" label={t('searchLabel')} help={t('fieldHelp')} defaultValue={t('searchValue')} />
        <Field id="invalid-example" label={t('invalidLabel')} help={t('invalidHelp')} error={t('invalidError')} defaultValue="" />
      </div>
      <div className="actions"><Button type="button" disabled aria-describedby="search-unavailable">{t('search')}</Button></div>
      <p id="search-unavailable" className="muted">{t('fieldHelp')}</p>
    </CardContent></Card>
    <StatusExamples />
    <UnavailableState title={t('unavailable')}>{t('disabledNote')}</UnavailableState>
  </div>
}
