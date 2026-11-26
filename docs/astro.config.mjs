import starlight from "@astrojs/starlight";
import { defineConfig } from "astro/config";

export default defineConfig({
  site: "https://dougmaitelli.github.io",
  base: "/Campfire",
  srcDir: ".",
  outDir: "./dist",
  redirects: { "/console": "/Campfire/servers/#console" },
  integrations: [
    starlight({
      title: "Campfire",
      description: "Host and manage game servers across your Linux machines.",
      logo: { src: "../web/public/brand/campfire.svg", alt: "Campfire" },
      favicon: "/favicon.png",
      customCss: ["./styles/docs.css"],
      components: { Hero: "./components/Hero.astro" },
      editLink: {
        baseUrl: "https://github.com/dougmaitelli/Campfire/edit/master/docs/",
      },
      lastUpdated: true,
      social: [
        {
          icon: "github",
          label: "Campfire on GitHub",
          href: "https://github.com/dougmaitelli/Campfire",
        },
      ],
      sidebar: [
        { label: "Overview", slug: "" },
        {
          label: "Getting started",
          items: [
            { label: "How Campfire works", slug: "concepts" },
            { label: "Docker Compose", slug: "getting-started" },
            { label: "Linux binary", slug: "binary-installation" },
            { label: "Your first server", slug: "first-server" },
          ],
        },
        {
          label: "Configuration",
          items: [
            { label: "Settings reference", slug: "configuration" },
            { label: "Accounts and access", slug: "authentication" },
            { label: "Notifications", slug: "notifications" },
            { label: "Public status pages", slug: "status-pages" },
            { label: "Discord integration", slug: "discord" },
          ],
        },
        {
          label: "Managing games",
          items: [
            { label: "Sparks", slug: "spark-agent" },
            { label: "Embers", slug: "embers" },
            { label: "Servers", slug: "servers" },
            { label: "Files and SFTP", slug: "server-files" },
            { label: "Schedules", slug: "schedules" },
            { label: "Backups", slug: "server-backups" },
            { label: "Copies and transfers", slug: "server-transfers" },
          ],
        },
        {
          label: "Operations",
          items: [
            { label: "Upgrades and recovery", slug: "operations" },
            { label: "Security and storage", slug: "security" },
          ],
        },
        {
          label: "Panel API",
          items: [
            { label: "Make your first request", slug: "api" },
            { label: "Integration guide", slug: "api/guide" },
            { label: "Servers and monitoring", slug: "api/servers" },
            { label: "Files and backups", slug: "api/storage" },
            { label: "Schedules, jobs, and audit", slug: "api/automation" },
            { label: "Notifications and publishing", slug: "api/publishing" },
            {
              label: "Administration and accounts",
              slug: "api/administration",
            },
          ],
        },
        {
          label: "Development",
          items: [
            { label: "Architecture", slug: "architecture" },
            { label: "Local development", slug: "development" },
            { label: "Ember schema", slug: "ember-schema" },
            { label: "Game integrations", slug: "game-integrations" },
          ],
        },
        { label: "Roadmap", slug: "roadmap" },
      ],
    }),
  ],
});
