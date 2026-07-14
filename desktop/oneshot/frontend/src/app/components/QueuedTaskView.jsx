import { Kicker } from "../../ui/primitives.jsx";
import { formatTime } from "../format.js";

export default function QueuedTaskView({ task, position }) {
  return <div className="queued-task-view"><div className="queue-orbit"><span>{position}</span></div><Kicker>workspace fifo</Kicker><h3>正在等待前面的任务</h3><p>同一个 Workspace 一次只激活一个排队任务；前序任务完成或终止后会自动启动。</p><dl><div><dt>任务目标</dt><dd>{task.prompt}</dd></div><div><dt>入队时间</dt><dd>{formatTime(task.queue?.enqueuedAt || task.createdAt)}</dd></div><div><dt>附件</dt><dd>{task.attachments?.length || 0} 个</dd></div></dl></div>;
}
