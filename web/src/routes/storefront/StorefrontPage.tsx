import type { Route } from '@/lib/route'
import { useSiteCopy } from '@/lib/site-copy'
import HomePage from './HomePage'
import AuctionIndexPage from './AuctionIndexPage'
import AuctionDetailPage from './AuctionDetailPage'
import LotIndexPage from './LotIndexPage'
import LotDetailPage from './LotDetailPage'
export default function StorefrontPage({ route }: { route: Route }) {
  const { t } = useSiteCopy()
  switch (route.page) {
    case 'home':
      return <HomePage />
    case 'auctions':
      return <AuctionIndexPage />
    case 'auctionDetail':
      return <AuctionDetailPage slug={route.slug!} />
    case 'lots':
      return <LotIndexPage />
    case 'lotDetail':
      return <LotDetailPage slug={route.slug!} />
    default:
      return (
        <section className="not-found stack">
          <h1>{t('notFound')}</h1>
          <p>{t('notFoundNote')}</p>
          <a href="/">{t('homeLink')}</a>
        </section>
      )
  }
}
