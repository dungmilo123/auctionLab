import { useRef } from 'react'
import { DropdownMenu } from 'radix-ui'
import { Check, ChevronDown, Globe } from 'lucide-react'
import { Icon } from './Icon'
import { useSiteCopy } from '@/lib/site-copy'

export default function LanguageMenu() {
  const { lang, setLang, t } = useSiteCopy()
  const trigger = useRef<HTMLButtonElement>(null)
  return <DropdownMenu.Root modal={false}>
    <DropdownMenu.Trigger ref={trigger} className="site-button site-button-ghost language-trigger" aria-label={`${t('language')}: ${lang === 'vi' ? 'Tiếng Việt' : 'English'}`}>
      <Icon icon={Globe} /><span lang={lang}>{lang === 'vi' ? 'Tiếng Việt' : 'English'}</span><Icon icon={ChevronDown} size={16} />
    </DropdownMenu.Trigger>
    <DropdownMenu.Portal>
      <DropdownMenu.Content className="website-overlay language-menu" sideOffset={8} align="end" loop onInteractOutside={event => {
        // A closing portal may still receive the trigger's next pointer/focus event.
        // The trigger owns toggling; an obsolete outside dismissal must not undo it.
        const target = event.detail.originalEvent.target
        if (target instanceof Node && trigger.current?.contains(target)) event.preventDefault()
      }}>
        <DropdownMenu.RadioGroup value={lang} onValueChange={value => { if (value === 'en' || value === 'vi') setLang(value) }}>
          {(['en', 'vi'] as const).map(code => <DropdownMenu.RadioItem key={code} value={code} className="language-option" lang={code}>
            <span>{code === 'en' ? 'English' : 'Tiếng Việt'}</span><DropdownMenu.ItemIndicator><Icon icon={Check} size={18} /></DropdownMenu.ItemIndicator>
          </DropdownMenu.RadioItem>)}
        </DropdownMenu.RadioGroup>
      </DropdownMenu.Content>
    </DropdownMenu.Portal>
  </DropdownMenu.Root>
}
