import starlight from "@astrojs/starlight";
import { defineConfig } from "astro/config";

export default defineConfig({
  site: "https://dougmaitelli.github.io",
  base: "/Campfire",
  srcDir: ".",
  outDir: "./dist",
  integrations: [
    starlight({
      title: "Campfire",
      description: "Host and manage game servers across your Linux machines.",
      logo: { src: "../web/public/favicon.svg", alt: "Campfire" },
      favicon: "/favicon.svg",
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
          ],
        },
        {
          label: "Managing games",
          items: [
            { label: "Sparks", slug: "spark-agent" },
            { label: "Embers and Egg imports", slug: "egg-compatibility" },
            { label: "Console", slug: "console" },
            { label: "Files and SFTP", slug: "server-files" },
            { label: "Schedules", slug: "schedules" },
            { label: "Backups", slug: "server-backups" },
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
          label: "Development",
          items: [
            { label: "Local development", slug: "development" },
            { label: "Architecture", slug: "architecture" },
          ],
        },
      ],
    }),
  ],
});
