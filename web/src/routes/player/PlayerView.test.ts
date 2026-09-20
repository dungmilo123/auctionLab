/// <reference types="node" />
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const playerViewSource = readFileSync(fileURLToPath(new URL('./PlayerView.tsx', import.meta.url)), 'utf8')

describe('the shared player budget readout', () => {
  it('uses the latest non-null snapshot budget and approved copy', () => {
    expect(playerViewSource).toMatch(/import \{ formatUSD \} from ['"]@\/lib\/format['"]/) 
    expect(playerViewSource).toMatch(/view\.me !== null/) 
    expect(playerViewSource).toMatch(/formatUSD\(view\.me\.budget\)/)
    expect(playerViewSource).toMatch(/t\(['"]lobby\.budgetLabel['"]\)/)
    expect(playerViewSource).toMatch(/t\(['"]lobby\.budgetCaption['"]\)/)
  })
})
