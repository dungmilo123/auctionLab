import { useRef } from 'react'
import { DropdownMenu } from 'radix-ui'
import { Check, ChevronDown, Globe } from 'lucide-react'
import { cn } from 'cn'
import { Icon } from './Icon'
import { buttonVariants } from '@/components/ui/button-variants'
import { useSiteCopy } from '@/lib/site-copy'

export default function LanguageMenu() {
  const { lang, setLang, t } = useSiteCopy()
  const trigger = useRef<HTMLButtonElement>(null)
  return <DropdownMenu.Root modal={false}>
    <DropdownMenu.Trigger ref={trigger} className={cn(buttonVariants({ variant: 'ghost' }), 'whitespace-nowrap px-2.5')} aria-label={`${t('language')}: ${lang === 'vi' ? 'Tiếng Việt' : 'English'}`}>
      <Icon icon={Globe} /><span lang={lang}>{lang === 'vi' ? 'VI' : 'EN'}</span><Icon icon={ChevronDown} size={16} />
    </DropdownMenu.Trigger>
    <DropdownMenu.Portal>
      <DropdownMenu.Content className="z-[70] min-w-[200px] max-w-[calc(100vw-32px)] animate-[panel-in_240ms_cubic-bezier(.22,1,.36,1)] rounded-[2px] border border-hairline bg-paper p-1.5 text-ink shadow-[0_18px_44px_#11111126]" sideOffset={8} align="end" loop onInteractOutside={event => {
        // A closing portal may still receive the trigger's next pointer/focus event.
        // The trigger owns toggling; an obsolete outside dismissal must not undo it.
        const target = event.detail.originalEvent.target
        if (target instanceof Node && trigger.current?.contains(target)) event.preventDefault()
      }}>
        <DropdownMenu.RadioGroup value={lang} onValueChange={value => { if (value === 'en' || value === 'vi') setLang(value) }}>
          {(['en', 'vi'] as const).map(code => <DropdownMenu.RadioItem key={code} value={code} className="flex min-h-11 cursor-pointer items-center justify-between gap-6 rounded-[2px] px-3 py-2.5 font-mono text-xs tracking-[0.06em] uppercase data-[highlighted]:bg-background data-[highlighted]:outline-2 data-[highlighted]:-outline-offset-2 data-[highlighted]:outline-ink" lang={code}>
            <span>{code === 'en' ? 'English' : 'Tiếng Việt'}</span><DropdownMenu.ItemIndicator><Icon icon={Check} size={18} /></DropdownMenu.ItemIndicator>
          </DropdownMenu.RadioItem>)}
        </DropdownMenu.RadioGroup>
      </DropdownMenu.Content>
    </DropdownMenu.Portal>
  </DropdownMenu.Root>
}
