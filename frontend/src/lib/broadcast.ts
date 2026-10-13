/**
 * xterm.js reports a terminal's automatic replies through the same onData
 * channel as keystrokes. Those belong to the pane that was asked, so they
 * must never be broadcast to other sessions.
 */
const REPORTS: RegExp[] = [
  /^\x1b\[\d+;\d+R$/, // cursor position report (reply to CSI 6 n)
  /^\x1b\[\?[\d;]*[cn]$/, // primary device attributes / DEC status reports
  /^\x1b\[>[\d;]*c$/, // secondary device attributes
  /^\x1b\[\d*(?:;\d*)*n$/, // device status report reply
  /^\x1b\[[IO]$/, // focus in / out
  /^\x1b\[M[\s\S]{3}$/, // X10 mouse report
  /^\x1b\[<\d+;\d+;\d+[Mm]$/, // SGR mouse report
  /^\x1b\][\s\S]*(?:\x07|\x1b\\)$/, // OSC reply (e.g. colour queries)
  /^\x1bP[\s\S]*\x1b\\$/, // DCS reply (e.g. DECRQSS)
  /^\x1b\[\d+;\d+;\d+t$/, // window size reports
]

/** Whether data is a terminal-generated reply rather than user input. */
export function isTerminalReport(data: string): boolean {
  return REPORTS.some((re) => re.test(data))
}
