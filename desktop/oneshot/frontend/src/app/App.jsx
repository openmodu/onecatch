export default function App() {
  return (
    <main className="shell">
      <aside className="sidebar">
        <div className="brand">Oneshot</div>
        <nav>
          <a className="active" href="#agents">Agents</a>
          <a href="#orders">Orders</a>
          <a href="#billing">Billing</a>
        </nav>
      </aside>
      <section className="workspace">
        <header>
          <p>Desktop workspace</p>
          <h1>Agent service marketplace</h1>
        </header>
        <div className="panel">
          <h2>Wails v3 scaffold</h2>
          <p>
            This frontend is ready for the prototype UI migration. The Go bindings already
            delegate to the shared backend services.
          </p>
        </div>
      </section>
      <aside className="inspector">
        <h2>Runtime</h2>
        <p>Local development services are wired with an in-memory repository.</p>
      </aside>
    </main>
  );
}
