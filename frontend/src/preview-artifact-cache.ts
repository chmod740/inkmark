import type { PreviewImageResourceSet } from './image-resources'

export const maximumCachedPreviewArtifacts = 6

export interface PreviewArtifact {
  readonly tabID: string
  readonly key: string
  readonly fragment: DocumentFragment
  readonly images: PreviewImageResourceSet
  readonly disposeExtendedDiagrams: (() => void) | null
  readonly needsEnhancement: boolean
}

/** LRU ownership store for inactive preview DOM trees and their Blob URLs. */
export class PreviewArtifactCache {
  private readonly entries = new Map<string, PreviewArtifact>()

  get(tabID: string, key: string) {
    const artifact = this.entries.get(tabID)
    if (!artifact) return null
    if (artifact.key !== key) {
      this.drop(tabID)
      return null
    }
    this.entries.delete(tabID)
    return artifact
  }

  put(artifact: PreviewArtifact) {
    this.drop(artifact.tabID)
    this.entries.set(artifact.tabID, artifact)
    while (this.entries.size > maximumCachedPreviewArtifacts) {
      const oldest = this.entries.keys().next().value as string | undefined
      if (!oldest) break
      this.drop(oldest)
    }
  }

  drop(tabID: string) {
    const artifact = this.entries.get(tabID)
    if (!artifact) return
    this.entries.delete(tabID)
    artifact.disposeExtendedDiagrams?.()
    artifact.images.release()
  }

  clear() {
    Array.from(this.entries.keys()).forEach((tabID) => this.drop(tabID))
  }

  get size() {
    return this.entries.size
  }
}
