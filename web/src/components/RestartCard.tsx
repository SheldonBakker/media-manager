import { useCallback, useEffect, useState } from 'react'
import { api, type SystemControl, type SystemOptions } from '../api'
import { timeAgo } from '../format'
import { t, tx, st } from '../i18n'
import { reloadWhenBack } from '../reloadWhenBack'
import '../restart.css'
import { useConfirm } from './ConfirmProvider'
import Icon from './Icon'
import Loading from './Loading'
import Switch from './Switch'
import { useToast } from './Toast'

// ---- Settings > System: restart ----

export function RestartCard() {
  const toast = useToast()
  const confirm = useConfirm()
  const [control, setControl] = useState<SystemControl | null>(null)
  const [options, setOptions] = useState<SystemOptions | null>(null)
  const [waiting, setWaiting] = useState(false)
  const load = useCallback(
    () =>
      Promise.all([api.systemControl(), api.systemOptions()])
        .then(([c, o]) => {
          setControl(c)
          setOptions(o)
        })
        .catch(() => undefined),
    [],
  )
  useEffect(() => {
    void load()
  }, [load])

  async function restart(safe: boolean) {
    const ok = await confirm(
      safe
        ? {
            title: t('Restart with automation paused?'),
            body: (
              <p>
                {t('Restarts once with automatic searching, downloading and refreshing paused, so you can fix a setting. The next restart turns everything back on. Downloads in progress stop and can be resumed afterwards.')}
              </p>
            ),
            confirmLabel: t('Restart paused'),
          }
        : {
            title: t('Restart Mediarium?'),
            body: <p>{t('Mediarium is unavailable for about half a minute. Downloads in progress stop and can be resumed afterwards.')}</p>,
            confirmLabel: t('Restart'),
          },
    )
    if (!ok) return
    try {
      await api.restartApp(safe)
      toast.success(t('Restarting…'))
      setWaiting(true)
      const back = await reloadWhenBack()
      if (!back) {
        setWaiting(false)
        toast.info(t("Mediarium didn't come back. Start it again, then reload this page."))
      }
    } catch (e) {
      toast.error(e instanceof Error ? e.message : String(e))
    }
  }

  async function shutDown() {
    const ok = await confirm({
      title: t('Shut down Mediarium?'),
      body: <p>{t('Mediarium stops, and nothing starts it again. To use it again, start it yourself.')}</p>,
      confirmLabel: t('Shut down'),
      danger: true,
    })
    if (!ok) return
    try {
      await api.shutdownApp()
      toast.success(t('Mediarium is shutting down.'))
    } catch (e) {
      toast.error(e instanceof Error ? e.message : String(e))
    }
  }

  async function setStuck(v: boolean) {
    try {
      setOptions(await api.putSystemOptions({ autoRestartWhenStuck: v }))
      toast.success(v ? t('Restart if stuck is on.') : t('Restart if stuck is off.'))
    } catch (e) {
      toast.error(e instanceof Error ? e.message : String(e))
    }
  }

  const c = control
  return (
    <fieldset className="group torrent">
      <legend>
        <Icon name="refresh" size={14} /> {t('Restart')}
      </legend>
      {!c || !options ? (
        <Loading height={70} />
      ) : (
        <>
          <p style={{ marginTop: 0 }}>{st(c.note)}</p>
          {c.selfRestart && (
            <div className="restart-note" role="alert">
              <p>
                {tx('{restarted} {when} because it stopped answering.', { restarted: <strong>{t('Mediarium restarted itself')}</strong>, when: timeAgo(c.selfRestart.at) })}
              </p>
            </div>
          )}
          <div className="control-actions">
            <button className="primary btn-with-icon" onClick={() => void restart(false)} disabled={!c.canRestart || waiting}>
              <Icon name="refresh" size={15} /> {waiting ? t('Restarting…') : t('Restart Mediarium')}
            </button>
            <button className="btn-with-icon" onClick={() => void restart(true)} disabled={!c.canRestart || waiting}>
              <Icon name="pause" size={15} /> {t('Restart with automation paused')}
            </button>
            {c.canShutdown && (
              <button className="danger-ghost btn-with-icon" onClick={() => void shutDown()}>
                <Icon name="stop" size={15} /> {t('Shut down')}
              </button>
            )}
          </div>
          {!c.canRestart && <p className="error-text" style={{ marginTop: 0 }}>{t("Restart isn't available here. Start Mediarium again yourself.")}</p>}
          <Switch
            checked={options.autoRestartWhenStuck}
            onChange={(v) => void setStuck(v)}
            label={t('Restart by itself if it stops answering')}
            showState
            description={t('After three minutes without an answer. Never during a restore.')}
          />
        </>
      )}
    </fieldset>
  )
}
