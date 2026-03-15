import type { IntegrationProvider } from '../types'
import { Button } from './Button'
import { Footer } from './Footer'
import { IntegrationProviderLogo } from './IntegrationProviderLogo'
import { CheckIcon } from './icons'

interface Props {
  integration: IntegrationProvider
  onDone: () => void
  onManage: () => void
}

export function IntegrationSuccess({ integration, onDone, onManage }: Props) {
  const name = integration.display_name || integration.provider || ''

  return (
    <div className="flex flex-col items-center justify-center h-full py-7 px-12 gap-4">
      <div className="flex items-center justify-center rounded-full bg-cw-success-bg shrink-0 size-16">
        <IntegrationProviderLogo 
          providerName={integration.provider ?? ''} 
          size="size-10" 
          rounded="rounded-lg"
        />
      </div>
      <div className="flex items-center justify-center -mt-2">
        <div className="flex items-center justify-center rounded-full bg-cw-success shrink-0 size-6">
          <CheckIcon size={16} className="text-white" />
        </div>
      </div>
      
      <div className="text-xl text-cw-heading font-bold leading-6">Connected</div>
      <div className="text-sm text-center leading-normal text-cw-secondary">
        {name} has been connected successfully.
      </div>
      
      <div className="flex flex-col w-full gap-2 mt-3">
        <Button onClick={onManage} variant="primary">
          Manage connection
        </Button>
        <Button onClick={onDone} variant="secondary">
          Done
        </Button>
      </div>
      
      <Footer />
    </div>
  )
}
