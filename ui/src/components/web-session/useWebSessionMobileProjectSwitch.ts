import { computed, ref, toValue, type MaybeRefOrGetter } from 'vue';
import { useProjectStore } from '@/stores/project';
import { buildProjectBadgeMap, type ProjectBadge } from '@/utils/projectBadge';

const MAX_PROJECT_SWITCH_ITEMS = 10;

export type WebSessionMobileProjectSwitchOptions = {
  projectId: MaybeRefOrGetter<string>;
  getProjectName: (projectId: string) => string;
};

export function useWebSessionMobileProjectSwitch({
  projectId,
  getProjectName,
}: WebSessionMobileProjectSwitchOptions) {
  const projectStore = useProjectStore();
  const search = ref('');
  const badges = computed(() => {
    const currentProjectId = toValue(projectId);
    const ordered = [
      ...projectStore.recentProjects.map(project => project.id),
      currentProjectId,
    ].filter(
      (itemProjectId, index, items) =>
        Boolean(itemProjectId) && items.indexOf(itemProjectId) === index
    );
    return buildProjectBadgeMap(ordered, getProjectName);
  });
  const currentBadge = computed<ProjectBadge | null>(
    () => badges.value.get(toValue(projectId)) ?? null
  );
  const filteredProjects = computed(() => {
    const query = search.value.trim().toLocaleLowerCase();
    const projects = query
      ? projectStore.recentProjects.filter(project =>
          [project.name, project.path, project.id].some(value =>
            value?.toLocaleLowerCase().includes(query)
          )
        )
      : projectStore.recentProjects;
    return projects.slice(0, MAX_PROJECT_SWITCH_ITEMS);
  });

  return {
    badges,
    currentBadge,
    filteredProjects,
    search,
  };
}
